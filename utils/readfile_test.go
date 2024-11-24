package utils

import (
	"os"
	"testing"
)

func TestReadBook(t *testing.T) {
	// 正常情况测试
	content, err := ReadBook("test.txt")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	expectedContent := "这是测试文件的内容"
	if content != expectedContent {
		t.Errorf("Expected content %s, got %s", expectedContent, content)
	}

	// 文件不存在测试
	_, err = ReadBook("nonexistent.txt")
	if err == nil {
		t.Errorf("Expected error for nonexistent file, got nil")
	}

	// 读取空文件测试
	emptyFile, err := os.Create("empty.txt")
	if err != nil {
		t.Fatalf("Error creating empty file: %v", err)
	}
	emptyFile.Close()
	content, err = ReadBook("empty.txt")
	if err != nil {
		t.Errorf("Expected no error for empty file, got %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty content for empty file, got %s", content)
	}

	// 编码错误测试
	wrongEncodingFile, err := os.Create("wrong_encoding.txt")
	if err != nil {
		t.Fatalf("Error creating wrong encoding file: %v", err)
	}
	_, err = wrongEncodingFile.WriteString("这是错误编码的内容")
	if err != nil {
		t.Fatalf("Error writing to wrong encoding file: %v", err)
	}
	wrongEncodingFile.Close()
	_, err = ReadBook("wrong_encoding.txt")
	if err == nil {
		t.Errorf("Expected error for wrong encoding, got nil")
	}

}

func TestReadRemoteBook(t *testing.T) {
	// 正常情况测试
	filenameURL := "http://cdn.novel.lovetalk.chat/%E3%80%90%E7%8B%AC%E5%AE%B6%E6%95%B4%E7%90%86%E3%80%91%E5%8F%B2%E8%AF%97%E7%BA%A7%EF%BD%9C%E5%B0%8F%E8%AF%B4%E5%A4%A7%E5%90%88%E9%9B%86%20%E2%91%A0/A1%7E%E3%80%90%E5%B0%8F%E8%AF%B4%E5%A4%A7%E5%90%88%E9%9B%86%E3%80%91%E6%89%93%E5%8C%85%E7%B3%BB%E5%88%97%EF%BC%88%E5%86%85%E5%90%AB100%2B%E4%B8%AA%E7%BB%86%E5%88%86%E7%B1%BB%E7%9B%AE%EF%BC%89%E7%B2%BE%E5%BF%83%E6%95%B4%E7%90%86%EF%BD%9C%E5%85%A8txt%E6%A0%BC%E5%BC%8F/%E3%80%90%E6%9C%AA%E5%88%86%E7%B1%BB%E3%80%91%E5%B0%8F%E8%AF%B4%E6%89%93%E5%8C%85%E5%90%88%E9%9B%86%E2%91%A4%EF%BC%88%E5%85%A8txt%E6%A0%BC%E5%BC%8F%EF%BC%89/25%E5%B0%8F%E6%97%B6.txt"

	_, err := ReadRemoteBook(filenameURL)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}
