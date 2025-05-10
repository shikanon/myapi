package cloudsdk

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNonStreamSynth_Success 测试成功的语音合成场景
func TestNonStreamSynth_Success(t *testing.T) {
	appid := os.Getenv("APPID")
	apiKey := os.Getenv("APIKEY")
	apiSecret := os.Getenv("APISECRET")
	// 准备测试数据
	testText := "大家好，我给大家讲一个我爷爷给我讲过的故事，关于一种神奇的中草药——益母草。\n很久以前，在一个小村庄里，住着一位年轻的母亲，名叫阿莲。阿莲刚生完孩子，身体非常虚弱，常常感到头晕目眩，四肢无力。村里的老中医告诉她，她需要一种叫做益母草的草药来调理身体。然而，益母草生长在深山老林里，采摘非常困难。"
	testVoiceType := os.Getenv("VOICE_TYPE")
	testOutFile := "test_output.mp3"

	// 创建测试客户端并注入模拟连接
	client := NewTTSWsClient(appid, apiKey, apiSecret)

	// 执行测试
	err := client.NonStreamSynth(testText, testVoiceType, testOutFile)

	// 验证结果
	assert.NoError(t, err)

	// 检查是否创建了输出文件
	_, err = os.Stat(testOutFile)
	assert.NoError(t, err)

}
