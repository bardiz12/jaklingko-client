package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// DefaultSalt is the salt configured in com.jaklingko.core.configuration.api.b
	DefaultSalt = "@Pu557D35t120y312:v"
	// KeyIterations is the PBKDF2 iteration count
	KeyIterations = 10000
	// KeyLength is 256 bits (32 bytes)
	KeyLength = 32
)

// CipherEngine handles JakLingko request payload encryption and response decryption
type CipherEngine struct {
	key []byte
	iv  []byte
}

// NewCipherEngine creates a new CipherEngine using the JakLingko salt
func NewCipherEngine(salt string) *CipherEngine {
	if salt == "" {
		salt = DefaultSalt
	}
	derivedKey := pbkdf2HmacSha256([]byte(salt), []byte(salt), KeyIterations, KeyLength)
	
	// Convert derived key to URL-safe Base64 without padding, then decode back (identical to Android app)
	keyB64URL := Base64UrlEncode(derivedKey)
	key, _ := Base64UrlDecode(keyB64URL)

	return &CipherEngine{
		key: key,
		iv:  make([]byte, 16), // 16 zero bytes
	}
}

// EncryptPayload encrypts a plaintext JSON string into the JakLingko {"request": "..."} envelope
func (c *CipherEngine) EncryptPayload(plainJSON string) (string, error) {
	// 1. Base64 URL-safe encode plaintext
	strC := Base64UrlEncode([]byte(plainJSON))

	// 2. PKCS#7 pad strC bytes to multiple of 16 bytes
	padded := pkcs7Pad([]byte(strC), aes.BlockSize)

	// 3. AES-256-CBC encrypt
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", fmt.Errorf("create cipher failed: %w", err)
	}

	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, c.iv)
	mode.CryptBlocks(ciphertext, padded)

	// 4. Base64 URL-safe encode ciphertext
	encryptedStr := Base64UrlEncode(ciphertext)

	// 5. Wrap in {"request": "..."}
	wrapper := map[string]string{
		"request": encryptedStr,
	}
	b, err := json.Marshal(wrapper)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// DecryptPayload decrypts a response body if it contains the {"response": "..."} wrapper
// If the body does not contain "response", it returns the original body unmodified.
func (c *CipherEngine) DecryptPayload(respBody []byte) ([]byte, error) {
	var wrapper map[string]interface{}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		// Not JSON, return as-is
		return respBody, nil
	}

	respCipherRaw, ok := wrapper["response"]
	if !ok {
		// No encrypted payload, return original
		return respBody, nil
	}

	respCipherStr, ok := respCipherRaw.(string)
	if !ok || respCipherStr == "" {
		return respBody, nil
	}

	// 1. Base64 URL-safe decode ciphertext
	ciphertext, err := Base64UrlDecode(respCipherStr)
	if err != nil {
		return nil, fmt.Errorf("decode response base64 failed: %w", err)
	}

	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext length not multiple of 16")
	}

	// 2. AES-256-CBC decrypt
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher failed: %w", err)
	}

	decryptedPadded := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, c.iv)
	mode.CryptBlocks(decryptedPadded, ciphertext)

	// 3. PKCS#7 unpad
	decryptedStrC, err := pkcs7Unpad(decryptedPadded)
	if err != nil {
		return nil, fmt.Errorf("unpad response failed: %w", err)
	}

	// 4. Base64 URL-safe decode the inner string to recover original JSON
	originalJSON, err := Base64UrlDecode(string(decryptedStrC))
	if err != nil {
		return nil, fmt.Errorf("decode inner payload base64 failed: %w", err)
	}

	// 5. Merge inner JSON into the outer JSON wrapper (removing "response" key)
	var innerMap map[string]interface{}
	if err := json.Unmarshal(originalJSON, &innerMap); err == nil {
		delete(wrapper, "response")
		for k, v := range innerMap {
			wrapper[k] = v
		}
		return json.Marshal(wrapper)
	}

	return originalJSON, nil
}

// Base64UrlEncode encodes data to Base64 URL-safe string without '=' padding
func Base64UrlEncode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "/", "_")
	return strings.TrimRight(encoded, "=")
}

// Base64UrlDecode decodes Base64 URL-safe string with or without padding
func Base64UrlDecode(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if pad := len(s) % 4; pad > 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64.StdEncoding.DecodeString(s)
}

// pkcs7Pad appends PKCS#7 padding
func pkcs7Pad(src []byte, blockSize int) []byte {
	padding := blockSize - (len(src) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

// pkcs7Unpad removes PKCS#7 padding
func pkcs7Unpad(src []byte) ([]byte, error) {
	length := len(src)
	if length == 0 {
		return nil, errors.New("empty input for unpadding")
	}
	padding := int(src[length-1])
	if padding > length || padding == 0 {
		return nil, errors.New("invalid padding size")
	}
	for i := length - padding; i < length; i++ {
		if src[i] != byte(padding) {
			return nil, errors.New("invalid padding byte")
		}
	}
	return src[:length-padding], nil
}

// pbkdf2HmacSha256 derives a key using PBKDF2 with HMAC-SHA256 (pure Go standard library)
func pbkdf2HmacSha256(password, salt []byte, iter, keyLen int) []byte {
	numBlocks := (keyLen + sha256.Size - 1) / sha256.Size
	var result []byte
	buf := make([]byte, 4)

	for block := 1; block <= numBlocks; block++ {
		binary.BigEndian.PutUint32(buf, uint32(block))
		h := hmac.New(sha256.New, password)
		h.Write(salt)
		h.Write(buf)
		u := h.Sum(nil)
		blockSum := make([]byte, len(u))
		copy(blockSum, u)

		for i := 1; i < iter; i++ {
			h.Reset()
			h.Write(u)
			u = h.Sum(nil)
			for j := 0; j < len(blockSum); j++ {
				blockSum[j] ^= u[j]
			}
		}
		result = append(result, blockSum...)
	}

	return result[:keyLen]
}
