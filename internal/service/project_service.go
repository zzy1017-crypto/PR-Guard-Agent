package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	projecthash "pr-guard-agent/pkg/hash"
	"pr-guard-agent/pkg/parser"

	"gorm.io/gorm"
)

// ProjectService 提供与项目相关的业务逻辑，包括上传项目、提取文件、计算哈希值等操作
type ProjectService struct {
	db          *gorm.DB
	projectRepo *repository.ProjectRepository
}

// UploadProjectResult 表示上传项目操作的结果，包括项目ID、项目名称和文件数量
type UploadProjectResult struct {
	ProjectID   uint   `json:"project_id"`
	ProjectName string `json:"project_name"`
	FileCount   int    `json:"file_count"`
}

// NewProjectService 创建一个新的ProjectService实例，接收一个数据库连接对象，并初始化项目仓库
func NewProjectService(db *gorm.DB) *ProjectService {
	return &ProjectService{
		db:          db,
		projectRepo: repository.NewProjectRepository(db),
	}
}

// UploadProject 处理上传项目的业务逻辑，接收项目名称和zip文件内容，进行验证、提取文件、计算哈希值，并将结果保存到数据库中
func (s *ProjectService) UploadProject(projectName string, zipContent []byte) (*UploadProjectResult, error) {
	//去除项目名称的前后空格，并检查是否为空，如果为空，则返回错误信息
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return nil, fmt.Errorf("project_name is required")
	}
	//检查上传的zip文件内容是否为空，如果为空，则返回错误信息
	if len(zipContent) == 0 {
		return nil, fmt.Errorf("uploaded zip file is empty")
	}

	//创建一个新的Project对象，并将项目名称赋值给它，然后调用项目仓库的Create方法将其保存到数据库中，如果保存失败，则返回错误信息
	project := &model.Project{Name: projectName}
	if err := s.projectRepo.Create(project); err != nil {
		return nil, fmt.Errorf("save project failed: %w", err)
	}

	//准备项目目录，目录路径为"data/projects/{projectID}"
	projectDir := filepath.Join("data", "projects", fmt.Sprintf("%d", project.ID))
	//确保项目目录存在，如果不存在，则创建该目录及其父目录，如果创建失败，则返回错误信息
	if err := os.RemoveAll(projectDir); err != nil {
		s.cleanupProject(project.ID, "") //确保在项目创建失败时清理数据库中的项目记录
		return nil, fmt.Errorf("prepare project dir failed: %w", err)
	}

	//将zip文件内容解压到项目目录中，并返回提取的文件列表，如果解压失败，则返回错误信息
	files, err := parser.ExtractFilteredZip(bytes.NewReader(zipContent), int64(len(zipContent)), projectDir)
	if err != nil {
		s.cleanupProject(project.ID, projectDir)
		return nil, fmt.Errorf("extract zip failed: %w", err)
	}

	//检查提取的文件列表是否为空，如果为空，则返回错误信息
	projectFiles, codeVersionHash, err := buildProjectFiles(project.ID, files)
	if err != nil {
		s.cleanupProject(project.ID, projectDir)
		return nil, err
	}

	//使用数据库事务将项目的代码版本哈希值和提取的文件信息保存到数据库中，如果保存失败，则回滚事务并清理项目目录
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		projectRepo := repository.NewProjectRepository(tx)
		projectFileRepo := repository.NewProjectFileRepository(tx)

		//更新项目的代码版本哈希值，如果更新失败，则返回错误信息
		if err := projectRepo.UpdateCodeVersionHash(project.ID, codeVersionHash); err != nil {
			return fmt.Errorf("update project hash failed: %w", err)
		}
		//批量保存提取的文件信息到数据库中，如果保存失败，则返回错误信息
		if err := projectFileRepo.BatchCreate(projectFiles); err != nil {
			return fmt.Errorf("save project files failed: %w", err)
		}

		return nil
	}); err != nil {
		s.cleanupProject(project.ID, projectDir)
		return nil, err
	}

	//返回上传项目的结果，包括项目ID、项目名称和文件数量
	return &UploadProjectResult{
		ProjectID:   project.ID,
		ProjectName: project.Name,
		FileCount:   len(projectFiles),
	}, nil
}

// buildProjectFiles 构建项目文件信息列表，并计算项目的代码版本哈希值，返回项目文件列表、代码版本哈希值和错误信息
func buildProjectFiles(projectID uint, files []parser.ExtractedFile) ([]model.ProjectFile, string, error) {
	projectFiles := make([]model.ProjectFile, 0, len(files))
	contentHashes := make([]string, 0, len(files))

	//遍历提取的文件列表，为每个文件计算内容哈希值
	for _, file := range files {
		contentHash, err := projecthash.SHA256File(file.FullPath)
		if err != nil {
			return nil, "", fmt.Errorf("calculate content hash failed for %s: %w", file.RelativePath, err)
		}

		//将文件的内容哈希值添加到contentHashes切片中
		contentHashes = append(contentHashes, contentHash)
		//将提取的文件信息转换为ProjectFile对象，并添加到projectFiles切片中
		projectFiles = append(projectFiles, model.ProjectFile{
			ProjectID:   projectID,
			FilePath:    file.RelativePath,
			FileType:    file.FileType,
			ContentHash: contentHash,
			Size:        file.Size,
		})
	}

	//将所有文件的内容哈希值连接成一个字符串，并计算该字符串的SHA256哈希值，作为项目的代码版本哈希值
	return projectFiles, projecthash.SHA256String(strings.Join(contentHashes, "")), nil
}

// cleanupProject 清理项目数据，包括删除数据库中的项目记录和删除项目目录
func (s *ProjectService) cleanupProject(projectID uint, projectDir string) {
	//删除数据库中的项目记录，如果删除失败，则忽略错误
	_ = s.projectRepo.DeleteByID(projectID)
	//删除项目目录及其内容，如果目录路径不为空，则尝试删除该目录及其所有子文件和子目录，如果删除失败，则忽略错误
	if projectDir != "" {
		_ = os.RemoveAll(projectDir)
	}
}
