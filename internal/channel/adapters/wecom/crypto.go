package wecom

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
)

func DecryptFileAES256CBC(ciphertext []byte, aesKeyBase64 string) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("ciphertext is empty")
	}
	key, err := base64.StdEncoding.DecodeString(aesKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode aes key failed: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid aes key length: %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext block size")
	}
	iv := key[:aes.BlockSize]
	out := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, ciphertext)
	plain, err := pkcs7Unpad(out, 32)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func pkcs7Unpad(data []byte, maxPad int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("pkcs7 payload is empty")
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > maxPad || pad > len(data) {
		return nil, fmt.Errorf("invalid pkcs7 padding length: %d", pad)
	}
	padding := bytes.Repeat([]byte{byte(pad)}, pad)
	if !bytes.Equal(data[len(data)-pad:], padding) {
		return nil, fmt.Errorf("invalid pkcs7 padding bytes")
	}
	return data[:len(data)-pad], nil
}
