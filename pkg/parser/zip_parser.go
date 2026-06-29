package parser

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ExtractedFile 表示从zip文件中提取的文件的信息，包括相对路径、完整路径、文件类型和大小
type ExtractedFile struct {
	RelativePath string
	FullPath     string
	FileType     string
	Size         int64
}

// ExtractFilteredZip 从zip文件中提取符合条件的文件到指定目录，并返回提取的文件信息列表
func ExtractFilteredZip(reader io.ReaderAt, size int64, destDir string) ([]ExtractedFile, error) {
	//打开zip文件，创建一个zip.Reader对象，用于读取zip文件的内容
	zr, err := zip.NewReader(reader, size)
	//如果打开zip文件失败，则返回错误信息
	if err != nil {
		return nil, fmt.Errorf("open zip failed: %w", err)
	}

	//创建目标目录，如果创建失败，则返回错误信息
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create extract dir failed: %w", err)
	}

	//获取目标目录的绝对路径，如果获取失败，则返回错误信息
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return nil, fmt.Errorf("resolve extract dir failed: %w", err)
	}

	//创建一个map，用于存储提取的文件信息，避免重复提取同一文件
	extracted := make(map[string]ExtractedFile)
	//遍历zip文件中的每个文件条目，进行路径清理、类型检查和提取操作
	for _, zipFile := range zr.File {
		//清理zip条目的路径，确保路径合法且不包含危险字符，如果清理失败，则返回错误信息
		relativePath, err := cleanZipEntryPath(zipFile.Name)
		if err != nil {
			return nil, err
		}

		//检查zip条目是否为目录或非普通文件，如果是，则跳过该条目
		if zipFile.FileInfo().IsDir() {
			continue
		}
		//检查zip条目是否为普通文件，如果不是，则跳过该条目
		if !zipFile.FileInfo().Mode().IsRegular() {
			continue
		}
		//检查zip条目的路径是否符合保留条件，如果不符合，则跳过该条目
		if !ShouldKeepFile(relativePath) {
			continue
		}

		//构建目标文件的完整路径，并确保该路径位于目标目录内，如果不在，则返回错误信息
		targetPath := filepath.Join(absDest, filepath.FromSlash(relativePath))
		//确保目标路径位于目标目录内，如果不在，则返回错误信息
		if err := ensurePathInsideDir(absDest, targetPath); err != nil {
			return nil, err
		}

		//检查目标路径是否已经存在，如果存在，则跳过该条目，避免覆盖已有文件
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf("create file dir failed: %w", err)
		}

		//提取zip条目到目标路径，并返回写入的字节数，如果提取失败，则返回错误信息
		written, err := extractZipFile(zipFile, targetPath)
		if err != nil {
			return nil, err
		}

		//将提取的文件信息存储到extracted map中，使用相对路径作为键，避免重复提取同一文件
		extracted[relativePath] = ExtractedFile{
			RelativePath: relativePath,
			FullPath:     targetPath,
			FileType:     FileType(relativePath),
			Size:         written,
		}
	}

	//将extracted map中的文件信息转换为切片，并按相对路径排序，确保返回的文件列表有序
	files := make([]ExtractedFile, 0, len(extracted))
	//遍历extracted map，将每个提取的文件信息添加到files切片中
	for _, file := range extracted {
		files = append(files, file)
	}
	//按相对路径排序提取的文件列表，确保返回的文件列表有序
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})

	return files, nil
}

// cleanZipEntryPath 清理zip条目的路径，确保路径合法且不包含危险字符，如果路径不合法，则返回错误信息
func cleanZipEntryPath(entryName string) (string, error) {
	//将zip条目的路径标准化，去除多余的斜杠和点，确保路径合法
	name := normalizeZipPath(entryName)
	//检查清理后的路径是否为空或为当前目录，如果是，则返回错误信息
	if name == "." || name == "" {
		return "", fmt.Errorf("invalid empty zip entry path")
	}
	//检查清理后的路径是否为绝对路径、上级目录或包含冒号，如果是，则返回错误信息，防止路径遍历攻击
	if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, ":") {
		return "", fmt.Errorf("invalid zip entry path: %s", entryName)
	}
	return name, nil
}

// ensurePathInsideDir 确保目标路径位于指定目录内，如果不在，则返回错误信息，防止路径遍历攻击
func ensurePathInsideDir(absDir, targetPath string) error {
	//获取目标路径的绝对路径，如果获取失败，则返回错误信息
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve target path failed: %w", err)
	}

	//检查目标路径是否位于指定目录内，如果不在，则返回错误信息，防止路径遍历攻击
	rel, err := filepath.Rel(absDir, absTarget)
	if err != nil {
		return fmt.Errorf("check target path failed: %w", err)
	}
	//检查相对路径是否为上级目录或绝对路径，如果是，则返回错误信息，防止路径遍历攻击
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("zip entry escapes target dir: %s", targetPath)
	}

	return nil
}

// extractZipFile 提取zip条目到目标路径，并返回写入的字节数，如果提取失败，则返回错误信息
func extractZipFile(zipFile *zip.File, targetPath string) (int64, error) {
	//打开zip条目，获取其内容的读取器，如果打开失败，则返回错误信息
	src, err := zipFile.Open()
	if err != nil {
		return 0, fmt.Errorf("open zip file failed: %w", err)
	}
	defer src.Close() //确保在函数返回时关闭zip条目的读取器，释放资源

	//创建目标文件，并将zip条目的内容写入目标文件，如果创建或写入失败，则返回错误信息
	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("create extracted file failed: %w", err)
	}
	defer dst.Close() //确保在函数返回时关闭目标文件，释放资源

	//将zip条目的内容写入目标文件，并返回写入的字节数，如果写入失败，则返回错误信息
	written, err := io.Copy(dst, src)
	if err != nil {
		return 0, fmt.Errorf("write extracted file failed: %w", err)
	}

	return written, nil
}
