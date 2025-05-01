package cloudsdk

import (
	"os"
	"testing"
)

func TestASRClient_StreamRecognition(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	accesskey := os.Getenv("VOLC_ACCESS_KEY")
	appkey := os.Getenv("VOLC_APP_KEY")

	config := &AsrConfig{
		SegDuration: 100,
		WsURL:       "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel",
		UID:         "test",
		Format:      "wav",
		Rate:        16000,
		Bits:        16,
		Channel:     1,
		Codec:       "raw",
		AuthMethod:  "none",
		Streaming:   true,
		AccessKey:   accesskey,
		AppKey:      appkey,
	}

	client := NewAsrWsClient(config)
	result, err := client.RecognizeStream("hello_test.wav")
	if err != nil {
		t.Fatalf("Recognition failed: %v", err)
	}

	t.Logf("Recognition result: %+v\n", result)
}
