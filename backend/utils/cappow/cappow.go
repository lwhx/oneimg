// Package cappow 是 Cap（cap-pow）工作量证明的本地实现。
//
// 协议与 capjs-core（github.com/tiagozip/cap）格式 1 保持一致：
//   - 挑战 token 为 HS256 签名的 JWT，payload 含 {n,c,s,d,exp,iat}
//   - 第 i 个谜题的 salt/target 由 token 确定性派生：
//     saltSeed   = fnv1aResume(fnv1a(token), strconv.Itoa(i))
//     targetSeed = fnv1aResume(saltSeed, "d")
//     salt, target 由 xorshift 风格 PRNG 展开为 hex
//   - 客户端寻找 nonce：sha256(salt + nonce) 的 hex 以 target 为前缀
//
// 服务端据此生成挑战、校验谜题，并签发一次性 redeem token 供登录使用。
package cappow

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 挑战参数上限（与 capjs-core 一致）。
const (
	MaxChallengeCount      = 1000
	MaxChallengeSize       = 256
	MaxChallengeDifficulty = 16

	defaultChallengeCount = 50
	defaultChallengeSize  = 32
	defaultChallengeTTL   = 10 * time.Minute
)

// jwtHeaderB64 固定 JWT 头部 base64url：{"alg":"HS256","typ":"JWT"}
const jwtHeaderB64 = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

// Challenge 挑战参数（与 widget 的 {challenge:{c,s,d}} 一致）。
type Challenge struct {
	C int `json:"c"`
	S int `json:"s"`
	D int `json:"d"`
}

// GenerateResult 生成挑战的返回结构（与 widget challenge 响应一致）。
type GenerateResult struct {
	Challenge Challenge `json:"challenge"`
	Token     string    `json:"token"`
	Expires   int64     `json:"expires"`
}

// --- 底层原语 ---

func fnv1a(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h += (h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24)
	}
	return h
}

func fnv1aResume(state uint32, s string) uint32 {
	h := state
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h += (h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24)
	}
	return h
}

// prngFromHash 将初始 hash 扩展为 length 个 hex 字符（与 capjs-core 一致）。
func prngFromHash(initialHash uint32, length int) string {
	state := initialHash
	var b strings.Builder
	for b.Len() < length {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		b.WriteString(fmt.Sprintf("%08x", state))
	}
	return b.String()[:length]
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func randomHex(bytesCount int) string {
	b := make([]byte, bytesCount)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// DeriveSecret 从应用级密钥派生出 cap-pow 的 HMAC 密钥。
// 与 Cap 的 “secret ≥16 bytes 且跨进程一致” 要求兼容，跨重启稳定。
func DeriveSecret(configSecret string) []byte {
	return hmacSHA256([]byte("oneimg-cappow-v1"), []byte(configSecret))
}

// --- JWT ---

func jwtSign(payload map[string]any, secret []byte) string {
	bodyBytes, _ := json.Marshal(payload)
	body := base64.RawURLEncoding.EncodeToString(bodyBytes)
	sigInput := jwtHeaderB64 + "." + body
	sig := base64.RawURLEncoding.EncodeToString(hmacSHA256(secret, []byte(sigInput)))
	return sigInput + "." + sig
}

func jwtVerify(token string, secret []byte) (map[string]any, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid jwt structure")
	}
	sigInput := parts[0] + "." + parts[1]
	expected := hmacSHA256(secret, []byte(sigInput))
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(actual) != len(expected) {
		return nil, errors.New("invalid signature")
	}
	if subtle.ConstantTimeCompare(expected, actual) != 1 {
		return nil, errors.New("signature mismatch")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid payload json")
	}
	return payload, nil
}

// JwtSigHex 返回 JWT 签名的 hex（用于一次性挑战重放防护）。
func JwtSigHex(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return ""
	}
	return hex.EncodeToString(sig)
}

// --- 挑战生成与校验 ---

// GenerateChallenge 生成一个格式 1 的 PoW 挑战（不含 instrumentation）。
func GenerateChallenge(secret []byte, difficulty, count, size int, ttl time.Duration) (*GenerateResult, error) {
	if count < 1 || count > MaxChallengeCount {
		return nil, fmt.Errorf("cappow: challengeCount 必须在 [1,%d]，收到 %d", MaxChallengeCount, count)
	}
	if size < 1 || size > MaxChallengeSize {
		return nil, fmt.Errorf("cappow: challengeSize 必须在 [1,%d]，收到 %d", MaxChallengeSize, size)
	}
	if difficulty < 1 || difficulty > MaxChallengeDifficulty {
		return nil, fmt.Errorf("cappow: challengeDifficulty 必须在 [1,%d]，收到 %d", MaxChallengeDifficulty, difficulty)
	}
	if ttl <= 0 {
		ttl = defaultChallengeTTL
	}
	now := time.Now().UnixMilli()
	expires := now + ttl.Milliseconds()
	payload := map[string]any{
		"n":   randomHex(25),
		"c":   count,
		"s":   size,
		"d":   difficulty,
		"exp": expires,
		"iat": now,
	}
	return &GenerateResult{
		Challenge: Challenge{C: count, S: size, D: difficulty},
		Token:     jwtSign(payload, secret),
		Expires:   expires,
	}, nil
}

// ValidateChallenge 校验客户端提交的谜题解答。返回是否通过及失败原因。
func ValidateChallenge(secret []byte, token string, solutions []int) (bool, string) {
	if token == "" {
		return false, "missing_token"
	}
	if len(solutions) == 0 {
		return false, "missing_solutions"
	}
	payload, err := jwtVerify(token, secret)
	if err != nil {
		return false, "invalid_token"
	}
	expF, ok := payload["exp"].(float64)
	if !ok || int64(expF) < time.Now().UnixMilli() {
		return false, "expired"
	}
	c := toInt(payload["c"])
	s := toInt(payload["s"])
	d := toInt(payload["d"])
	if c < 1 || c > MaxChallengeCount || s < 1 || s > MaxChallengeSize || d < 1 || d > MaxChallengeDifficulty {
		return false, "invalid_token"
	}
	if len(solutions) != c {
		return false, "invalid_solutions"
	}

	tokenFnv := fnv1a(token)
	for i := 0; i < c; i++ {
		idxStr := strconv.Itoa(i + 1)
		saltSeed := fnv1aResume(tokenFnv, idxStr)
		targetSeed := fnv1aResume(saltSeed, "d")
		salt := prngFromHash(saltSeed, s)
		target := prngFromHash(targetSeed, d)
		hash := sha256.Sum256([]byte(salt + strconv.Itoa(solutions[i])))
		hashHex := hex.EncodeToString(hash[:])
		if !strings.HasPrefix(hashHex, target) {
			return false, "invalid_solution"
		}
	}
	return true, ""
}

// SolveChallenge 仅供服务端测试/自检使用：对每个谜题暴力寻找满足前缀的 nonce。
func SolveChallenge(token string, ch Challenge, progress func(done, total int)) ([]int, error) {
	solutions := make([]int, ch.C)
	tokenFnv := fnv1a(token)
	for i := 0; i < ch.C; i++ {
		idxStr := strconv.Itoa(i + 1)
		saltSeed := fnv1aResume(tokenFnv, idxStr)
		targetSeed := fnv1aResume(saltSeed, "d")
		salt := prngFromHash(saltSeed, ch.S)
		target := prngFromHash(targetSeed, ch.D)
		nonce := 0
		for {
			hash := sha256.Sum256([]byte(salt + strconv.Itoa(nonce)))
			if strings.HasPrefix(hex.EncodeToString(hash[:]), target) {
				solutions[i] = nonce
				break
			}
			nonce++
			if nonce > 10_000_000 {
				return nil, fmt.Errorf("cappow: puzzle %d 求解超时", i+1)
			}
		}
		if progress != nil {
			progress(i+1, ch.C)
		}
	}
	return solutions, nil
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

// --- 一次性 redeem token 存储（内存） ---

// Store 保存 cap-pow 签发的一次性 redeem token 与已消费的挑战签名。
// 单进程内存实现，配合短 TTL（默认 20 分钟）使用；进程重启后在途 token 失效。
type Store struct {
	mu     sync.Mutex
	tokens map[string]time.Time // token -> 过期时间
	nonces map[string]time.Time // 挑战签名 hex -> 过期时间
}

func NewStore() *Store {
	return &Store{
		tokens: make(map[string]time.Time),
		nonces: make(map[string]time.Time),
	}
}

// IssueToken 签发一个 ttl 后过期的一次性 token，返回 token 与过期毫秒时间戳。
func (s *Store) IssueToken(ttl time.Duration) (string, int64) {
	if ttl <= 0 {
		ttl = 20 * time.Minute
	}
	token := randomHex(32)
	exp := time.Now().Add(ttl)
	s.mu.Lock()
	s.cleanupLocked()
	s.tokens[token] = exp
	s.mu.Unlock()
	return token, exp.UnixMilli()
}

// ConsumeToken 校验并一次性消费 redeem token。成功返回 true（已删除，不可复用）。
func (s *Store) ConsumeToken(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tokens[token]
	if !ok {
		return false
	}
	delete(s.tokens, token)
	return time.Now().Before(exp)
}

// ConsumeChallengeSig 对挑战 JWT 签名做一次性重放防护。
// 首次消费返回 true，重复返回 false。
func (s *Store) ConsumeChallengeSig(sigHex string, ttl time.Duration) bool {
	if sigHex == "" {
		return false
	}
	if ttl <= 0 {
		ttl = defaultChallengeTTL
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nonces[sigHex]; ok {
		return false
	}
	s.nonces[sigHex] = time.Now().Add(ttl)
	s.cleanupLocked()
	return true
}

func (s *Store) cleanupLocked() {
	now := time.Now()
	for k, exp := range s.tokens {
		if now.After(exp) {
			delete(s.tokens, k)
		}
	}
	for k, exp := range s.nonces {
		if now.After(exp) {
			delete(s.nonces, k)
		}
	}
}
