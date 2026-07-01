package parser

import "testing"

func TestParseDiffModifiedFile(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
-func old() {}
+func new() {}
+func added() {}
 func keep() {}
`

	files, err := ParseDiff(diffText)
	if err != nil {
		t.Fatalf("ParseDiff returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	file := files[0]
	if file.FilePath != "main.go" || file.ChangedType != DiffChangeModified {
		t.Fatalf("unexpected file summary: %+v", file)
	}
	if len(file.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(file.Hunks))
	}
	if len(file.Hunks[0].Lines) != 5 {
		t.Fatalf("expected 5 hunk lines, got %d", len(file.Hunks[0].Lines))
	}

	deleted := file.Hunks[0].Lines[1]
	if deleted.Type != DiffLineDeleted || deleted.OldLine != 2 || deleted.NewLine != 0 {
		t.Fatalf("unexpected deleted line: %+v", deleted)
	}
	added := file.Hunks[0].Lines[2]
	if added.Type != DiffLineAdded || added.OldLine != 0 || added.NewLine != 2 {
		t.Fatalf("unexpected added line: %+v", added)
	}
	context := file.Hunks[0].Lines[4]
	if context.Type != DiffLineContext || context.OldLine != 3 || context.NewLine != 4 {
		t.Fatalf("unexpected context line: %+v", context)
	}
}

func TestParseDiffAddedAndDeletedFiles(t *testing.T) {
	diffText := `diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1,2 @@
+package main
+func created() {}
diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package main
-func removed() {}
`

	files, err := ParseDiff(diffText)
	if err != nil {
		t.Fatalf("ParseDiff returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].ChangedType != DiffChangeAdded || files[0].FilePath != "new.go" {
		t.Fatalf("unexpected added file summary: %+v", files[0])
	}
	if files[1].ChangedType != DiffChangeDeleted || files[1].FilePath != "old.go" {
		t.Fatalf("unexpected deleted file summary: %+v", files[1])
	}
}

func TestParseDiffRejectsInvalidInput(t *testing.T) {
	if _, err := ParseDiff("not a diff"); err == nil {
		t.Fatal("expected error for invalid diff")
	}
}
