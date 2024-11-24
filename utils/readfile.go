package utils

import (
	"io"
	"net/http"
	"os"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ReadBook 函数用于读取书籍内容
// 接收一个文件名作为参数，输出书本内容
func ReadBook(filename string) (string, error) {
	// 打开文件
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	// 创建一个新的 Reader，用于将GBK文件内容转换为 UTF-8 编码
	utf8Reader := transform.NewReader(file, simplifiedchinese.GBK.NewDecoder())
	// 使用 ioutil.ReadAll 读取整个文件内容
	content, err := io.ReadAll(utf8Reader)
	if err != nil {
		return "", err
	}
	// 返回读取到的文件内容
	return string(content), nil
}

func ReadRemoteBook(filenameURL string) (string, error) {
	// 远程下载文件
	resp, err := http.Get(filenameURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// 创建一个新的 Reader，用于将GBK文件内容转换为 UTF-8 编码
	utf8Reader := transform.NewReader(resp.Body, simplifiedchinese.GBK.NewDecoder())
	// 使用 ioutil.ReadAll 读取整个文件内容
	content, err := io.ReadAll(utf8Reader)
	if err != nil {
		return "", err
	}
	// 返回读取到的文件内容
	return string(content), nil
}
