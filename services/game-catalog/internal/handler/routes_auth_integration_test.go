package handler

import (
	"bytes"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/utils"

	"github.com/zeromicro/go-zero/rest"
)

const testJWTSecret = "routes-integration-secret"

// newAuthedTestServer 启动真实的 go-zero rest 服务器（含 RegisterHandlers 路由注册），
// 返回 base URL 与签发令牌的工具函数。端口取自空闲端口，数据目录隔离到临时目录。
func newAuthedTestServer(t *testing.T) (string, func(claims utils.JWTClaims) string) {
	t.Helper()

	// 预占一个空闲端口再释放，供 go-zero 使用（go-zero 不暴露 listener 句柄）
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}

	var c config.Config
	c.Port = port
	c.DataSource.File = t.TempDir() + "/games.json"
	c.Auth.JWTSecret = testJWTSecret

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	RegisterHandlers(server, svcCtx)

	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)

	// 等待服务器就绪
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/games")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	sign := func(claims utils.JWTClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(testJWTSecret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return signed
	}
	return base, sign
}

func validRouteClaims() utils.JWTClaims {
	return utils.JWTClaims{
		UserId:   7,
		Username: "bob",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

// TestRoutes_PostGamesRequiresAuth 验证路由注册结果：
// POST /games 受 JWT 保护，GET 路由保持匿名公开。
func TestRoutes_PostGamesRequiresAuth(t *testing.T) {
	base, sign := newAuthedTestServer(t)

	postBody := []byte(`{"title":"Halo","description":"FPS classic","genres":["FPS"],"platforms":["Xbox"],"release_date":"2001-11-15","developer":"Bungie","publisher":"Microsoft","tags":["sci-fi"],"score":9.5,"cover_image":"https://example.com/halo.jpg"}`)

	// 1) 匿名 POST → 401
	resp, err := http.Post(base+"/games", "application/json", bytes.NewReader(postBody))
	if err != nil {
		t.Fatalf("anonymous POST: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous POST status = %d, want 401 (body %s)", resp.StatusCode, body)
	}

	// 2) 无效令牌 POST → 401
	req, _ := http.NewRequest(http.MethodPost, base+"/games", bytes.NewReader(postBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bad-token POST: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad-token POST status = %d, want 401 (body %s)", resp.StatusCode, body)
	}

	// 3) 匿名 GET 仍公开
	resp, err = http.Get(base + "/games")
	if err != nil {
		t.Fatalf("anonymous GET: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("anonymous GET /games status = %d, want 200 (body %s)", resp.StatusCode, body)
	}

	// 4) 有效令牌 POST → 进入业务处理器（200，返回创建的游戏）
	req, _ = http.NewRequest(http.MethodPost, base+"/games", bytes.NewReader(postBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sign(validRouteClaims()))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authed POST: %v", err)
	}
	body = readAll(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authed POST status = %d, want 200 (body %s)", resp.StatusCode, body)
	}
	if !bytes.Contains([]byte(body), []byte(`"title":"Halo"`)) {
		t.Fatalf("authed POST body = %s, want created game with title Halo", body)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.String()
}
