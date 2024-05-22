package httptools

import "net/http"

var (
	extToMimeTypeMap = map[string]string{
		".html":  "text/html",
		".css":   "text/css",
		".js":    "application/javascript",
		".json":  "application/json",
		".jpg":   "image/jpeg",
		".png":   "image/png",
		".gif":   "image/gif",
		".svg":   "image/svg+xml",
		".ico":   "image/x-icon",
		".webp":  "image/webp",
		".bmp":   "image/bmp",
		".tiff":  "image/tiff",
		".woff":  "application/font-woff",
		".woff2": "application/font-woff2",
		".ttf":   "application/font-ttf",
		".otf":   "application/font-otf",
		".eot":   "application/vnd.ms-fontobject",
		".mp3":   "audio/mpeg",
		".mp4":   "video/mp4",
		".m4v":   "video/x-m4v",
		".mov":   "video/quicktime",
		".webm":  "video/webm",
		".flv":   "video/x-flv",
		".swf":   "application/x-shockwave-flash",
		".pdf":   "application/pdf",
		".doc":   "application/msword",
		".docx":  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":   "application/vnd.ms-excel",
		".xlsx":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":   "application/vnd.ms-powerpoint",
		".pptx":  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".txt":   "text/plain",
		".rtf":   "application/rtf",
		".xml":   "text/xml",
		".zip":   "application/zip",
		".rar":   "application/x-rar-compressed",
		".7z":    "application/x-7z-compressed",
		".bz2":   "application/x-bzip2",
		".gz":    "application/x-gzip",
		".xz":    "application/x-xz",
		".tar":   "application/x-tar",
	}
	mimeTypeToExtMap = map[string]string{
		"text/html":                     ".html",
		"text/css":                      ".css",
		"application/javascript":        ".js",
		"application/json":              ".json",
		"image/jpeg":                    ".jpg",
		"image/png":                     ".png",
		"image/gif":                     ".gif",
		"image/svg+xml":                 ".svg",
		"image/x-icon":                  ".ico",
		"image/webp":                    ".webp",
		"image/bmp":                     ".bmp",
		"image/tiff":                    ".tiff",
		"image/x-tiff":                  ".tiff",
		"image/x-ms-bmp":                ".bmp",
		"application/font-woff":         ".woff",
		"application/font-woff2":        ".woff2",
		"application/font-ttf":          ".ttf",
		"application/font-otf":          ".otf",
		"application/vnd.ms-fontobject": ".eot",
		"audio/mpeg":                    ".mp3",
		"video/mp4":                     ".mp4",
		"video/x-m4v":                   ".m4v",
		"video/quicktime":               ".mov",
		"video/webm":                    ".webm",
		"video/x-flv":                   ".flv",
		"application/x-shockwave-flash": ".swf",
		"application/pdf":               ".pdf",
		"application/msword":            ".doc",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
		"application/vnd.ms-excel": ".xls",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
		"application/vnd.ms-powerpoint":                                             ".ppt",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
		"text/plain":                   ".txt",
		"application/rtf":              ".rtf",
		"text/xml":                     ".xml",
		"application/zip":              ".zip",
		"application/x-rar-compressed": ".rar",
		"application/x-7z-compressed":  ".7z",
		"application/x-bzip2":          ".bz2",
		"application/x-gzip":           ".gz",
		"application/x-xz":             ".xz",
		"application/x-tar":            ".tar",
	}
)

// TransformContentType2Ext
func TransformContentType2Ext(hdr http.Header) string {
	ct := hdr.Get("Content-Type")
	if ct == "" {
		return ""
	}

	ext, ok := mimeTypeToExtMap[ct]
	if !ok {
		return ""
	}
	return ext
}

// TransformExt2ContentType
func TransformExt2ContentType(ext string) string {
	mt, ok := extToMimeTypeMap[ext]
	if !ok {
		return ""
	}
	return mt
}
