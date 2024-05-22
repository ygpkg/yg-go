package encryptor

import "bytes"

// func pkcs5Padding(cipherText []byte, blockSize int) []byte {
// 	padding := blockSize - len(cipherText)%blockSize
// 	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
// 	return append(cipherText, padtext...)
// }

// func pkcs5UnPadding(plainText []byte) []byte {
// 	length := len(plainText)
// 	unpadding := int(plainText[length-1])
// 	return plainText[:(length - unpadding)]
// }

func pad(cipherText []byte, blockSize int) []byte {
	padding := blockSize - len(cipherText)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(cipherText, padText...)
}

func unpad(plainText []byte) []byte {
	length := len(plainText)
	unpadding := int(plainText[length-1])
	return plainText[:(length - unpadding)]
}
