package cloudsdk

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Train(t *testing.T) {
	// 创建一个临时MP3文件用于测试
	audioPath := "./wu_doc.mp3"

	// 创建客户端并覆盖基础URL
	appid := os.Getenv("APPID")
	token := os.Getenv("TOKEN")
	speakerid := os.Getenv("SPEAKERID")
	client := NewClient(appid, token)
	client.WithHTTPClient(&http.Client{Timeout: 30 * time.Second})

	// 执行测试
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Train(ctx, speakerid, audioPath)
	require.NoError(t, err)

	// 验证响应
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	t.Logf("Response: %+v", resp.Body)
}
