package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id 参数：OWASP 推荐的低内存档位，单次约 64MB / 数十毫秒。
const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// VerifyPassword 可接受的参数上界：防御被注入的恶意哈希
// （超大 m 会 OOM、p=0 会触发除零 panic，均不依赖调用方防范）。
const (
	maxArgonMemory  = 256 * 1024 // KiB，256 MB
	maxArgonTime    = 10
	maxArgonThreads = 16
	maxArgonSaltCap = 64
	maxArgonKeyCap  = 64
)

var errBadHashFormat = errors.New("密码哈希格式非法")

// HashPassword 生成 PHC 格式的 argon2id 哈希：$argon2id$v=19$m=..,t=..,p=..$salt$hash
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword 恒定时间比对；任何格式错误都返回 false，不泄露细节。
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}

	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	// 参数与长度必须落在本实现同量级的范围内，超界一律视为非法。
	if memory == 0 || memory > maxArgonMemory ||
		timeCost == 0 || timeCost > maxArgonTime ||
		threads == 0 || threads > maxArgonThreads {
		return false
	}
	if len(salt) == 0 || len(salt) > maxArgonSaltCap ||
		len(want) == 0 || len(want) > maxArgonKeyCap {
		return false
	}

	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
