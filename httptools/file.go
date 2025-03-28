package httptools

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

// PostFile 通过HTTP上传文件
func PostFile(cli *http.Client, url, formName, fileName, path string) (res *http.Response, err error) {
	req, err := NewPostFileRequest(url, formName, fileName, path)
	if err != nil {
		return nil, err
	}
	res, err = cli.Do(req)
	return
}

// NewPostFileRequest 创建一个文件上传请求
// 模拟<form ...><input name="file" type="file" />...</form>
// 参数说明
// url: 上传服务器URL
// formName: 对应<input>标签中的name
// fileName: 为form表单中的文件名
// path: 为实际要上传的本地文件路径
func NewPostFileRequest(url, formName, fileName, path string) (req *http.Request, err error) {
	fd, err := os.Open(path)
	if err != nil {
		return
	}
	defer fd.Close()
	return NewPostFileReaderRequest(url, formName, fileName, fd)
}

// NewPostFileReaderRequest 创建一个文件上传请求
// 模拟<form ...><input name="file" type="file" />...</form>
// 参数说明
// url: 上传服务器URL
// formName: 对应<input>标签中的name
// fileName: 为form表单中的文件名
// path: 为实际要上传的本地文件路径
func NewPostFileReaderRequest(url, formName, fileName string, f io.Reader) (req *http.Request, err error) {
	buf := new(bytes.Buffer) // caveat IMO dont use this for large files, \
	// create a tmpfile and assemble your multipart from there (not tested)
	w := multipart.NewWriter(buf)

	fw, err := w.CreateFormFile(formName, fileName) //这里的file必须和服务器端的FormFile一致
	if err != nil {
		return
	}

	// Write file field from file to upload
	_, err = io.Copy(fw, f)
	if err != nil {
		return
	}
	// Important if you do not close the multipart writer you will not have a
	// terminating boundry
	w.Close()

	req, err = http.NewRequest("POST", url, buf)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	return
}
