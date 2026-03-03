package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
)

const (
	saltSize  = 16
	nonceSize = 12
	keySize   = 32
)

func DeriveKey(password []byte, salt []byte) []byte {
	key := append(password, salt...)
	for i := 0; i < 100000; i++ {
		h := sha256.Sum256(key)
		key = h[:]
	}
	return key[:keySize]
}

func EncryptFile(path string, password []byte) error {
	plaintext, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	key := DeriveKey(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, saltSize+nonceSize+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	if err := os.WriteFile(path+".enc", out, 0600); err != nil {
		return err
	}
	return os.Remove(path)
}

func DecryptFile(path string, password []byte) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < saltSize+nonceSize {
		return nil, errors.New("file too short")
	}
	salt := data[:saltSize]
	nonce := data[saltSize : saltSize+nonceSize]
	ciphertext := data[saltSize+nonceSize:]
	key := DeriveKey(password, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("wrong password or corrupted file")
	}
	return plaintext, nil
}

func RestoreFile(encPath string, plaintext []byte) error {
	origPath := encPath[:len(encPath)-4]
	if err := os.WriteFile(origPath, plaintext, 0644); err != nil {
		return err
	}
	return os.Remove(encPath)
}

func IsEncrypted(path string) bool {
	_, err := os.Stat(path + ".enc")
	return err == nil
}
