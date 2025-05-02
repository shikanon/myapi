package cloudsdk

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
)

var (
	enumMessageType = map[byte]string{
		0x0b: "audio-only server response",
		0x0c: "frontend server response",
		0x0f: "error message from server",
	}
	enumMessageTypeSpecificFlags = map[byte]string{
		0: "no sequence number",
		1: "sequence number > 0",
		2: "last message from server (seq < 0)",
		3: "sequence number < 0",
	}
	enumMessageSerializationMethods = map[byte]string{
		0:  "no serialization",
		1:  "JSON",
		15: "custom type",
	}
	enumMessageCompression = map[byte]string{
		0:  "no compression",
		1:  "gzip",
		15: "custom compression method",
	}
)

const (
	optQuery  string = "query"
	optSubmit string = "submit"
)

type TTSWsClient struct {
	appid        string
	apptoken     string
	clusterid    string
	encoding     string  // 默认值"mp3"
	speed_ratio  float32 // 默认值1.0
	volume_ratio float32 // 默认值1.0
	pitch_ratio  string  // 默认值""
}

type synResp struct {
	Audio  []byte
	IsLast bool
}

// version: b0001 (4 bits)
// header size: b0001 (4 bits)
// message type: b0001 (Full client request) (4bits)
// message type specific flags: b0000 (none) (4bits)
// message serialization method: b0001 (JSON) (4 bits)
// message compression: b0001 (gzip) (4bits)
// reserved data: 0x00 (1 byte)
var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

// NewTTSWsClient 创建新的TTS客户端实例
func NewTTSWsClient(appid, apptoken, clusterid string) *TTSWsClient {
	return &TTSWsClient{
		appid:        appid,
		apptoken:     apptoken,
		clusterid:    clusterid,
		encoding:     "mp3",
		speed_ratio:  1.0,
		volume_ratio: 1.0,
		pitch_ratio:  "",
	}
}

func (t *TTSWsClient) SetupInput(text, voiceType, opt string) (jsonParams []byte, err error) {
	reqID := uuid.Must(uuid.NewV4(), err).String()
	params := map[string]map[string]interface{}{
		"app": {
			"appid":   t.appid,
			"token":   t.apptoken,
			"cluster": t.clusterid,
		},
		"user": {
			"uid": reqID,
		},
		"audio": {
			"voice_type":   voiceType,
			"encoding":     t.encoding,
			"speed_ratio":  t.speed_ratio,
			"volume_ratio": t.volume_ratio,
			"pitch_ratio":  t.pitch_ratio,
		},
		"request": {
			"reqid":     reqID,
			"text":      text,
			"text_type": "plain",
			"operation": opt,
		},
	}

	return json.Marshal(params)
}

func gzipCompress(input []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty input")
	}

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	defer w.Close()

	if _, err := w.Write(input); err != nil {
		return nil, fmt.Errorf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %v", err)
	}
	return b.Bytes(), nil
}

func gzipDecompress(input []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty input")
	}

	b := bytes.NewBuffer(input)
	r, err := gzip.NewReader(b)
	if err != nil {
		return nil, fmt.Errorf("gzip reader create failed: %v", err)
	}
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("gzip read failed: %v", err)
	}
	return out, nil
}

func (t *TTSWsClient) parseResponse(res []byte) (synResp, error) {
	resp := synResp{}
	if len(res) < 4 {
		return resp, errors.New("response too short, minimum 4 bytes required")
	}

	protoVersion := res[0] >> 4
	headSize := res[0] & 0x0f
	messageType := res[1] >> 4
	messageTypeSpecificFlags := res[1] & 0x0f
	serializationMethod := res[2] >> 4
	messageCompression := res[2] & 0x0f
	reserve := res[3]

	// 调试信息输出
	fmt.Printf("            Protocol version: %x - version %d\n", protoVersion, protoVersion)
	fmt.Printf("                 Header size: %x - %d bytes\n", headSize, headSize*4)
	fmt.Printf("                Message type: %x - %s\n", messageType, enumMessageType[messageType])
	fmt.Printf(" Message type specific flags: %x - %s\n", messageTypeSpecificFlags,
		enumMessageTypeSpecificFlags[messageTypeSpecificFlags])
	fmt.Printf("Message serialization method: %x - %s\n",
		serializationMethod, enumMessageSerializationMethods[serializationMethod])
	fmt.Printf("         Message compression: %x - %s\n",
		messageCompression, enumMessageCompression[messageCompression])
	fmt.Printf("                    Reserved: %d\n", reserve)

	if headSize != 1 && len(res) >= int(headSize*4) {
		headerExtensions := res[4 : headSize*4]
		fmt.Printf("           Header extensions: % x\n", headerExtensions)
	}

	if len(res) < int(headSize*4) {
		return resp, fmt.Errorf("invalid header size, expected %d bytes but got %d",
			headSize*4, len(res))
	}
	payload := res[headSize*4:]

	switch messageType {
	case 0x0b: // audio-only server response
		if messageTypeSpecificFlags == 0 {
			fmt.Println("                Payload size: 0")
		} else {
			if len(payload) < 8 {
				return resp, fmt.Errorf("audio response too short, expected 8 bytes but got %d",
					len(payload))
			}
			sequenceNumber := int32(binary.BigEndian.Uint32(payload[0:4]))
			payloadSize := int32(binary.BigEndian.Uint32(payload[4:8]))
			payload = payload[8:]

			resp.Audio = append(resp.Audio, payload...)
			fmt.Printf("             Sequence number: %d\n", sequenceNumber)
			fmt.Printf("                Payload size: %d\n", payloadSize)

			if sequenceNumber < 0 {
				resp.IsLast = true
			}
		}

	case 0x0f: // error message
		if len(payload) < 8 {
			return resp, fmt.Errorf("error message too short, expected 8 bytes but got %d",
				len(payload))
		}
		code := int32(binary.BigEndian.Uint32(payload[0:4]))
		errMsg := payload[8:]

		if messageCompression == 1 {
			decompressed, err := gzipDecompress(errMsg)
			if err != nil {
				return resp, fmt.Errorf("error message decompress failed: %v", err)
			}
			errMsg = decompressed
		}

		fmt.Printf("                  Error code: %d\n", code)
		fmt.Printf("                   Error msg: %q\n", string(errMsg))
		return resp, fmt.Errorf("server error %d: %s", code, string(errMsg))

	case 0x0c: // frontend server response
		if len(payload) < 4 {
			return resp, fmt.Errorf("frontend message too short, expected 4 bytes but got %d",
				len(payload))
		}
		msgSize := int32(binary.BigEndian.Uint32(payload[0:4]))
		payload = payload[4:]

		if messageCompression == 1 {
			decompressed, err := gzipDecompress(payload)
			if err != nil {
				return resp, fmt.Errorf("frontend message decompress failed: %v", err)
			}
			payload = decompressed
		}

		fmt.Printf("            Frontend message: %q\n", string(payload))
		fmt.Printf("                 Message size: %d\n", msgSize)

	default:
		if _, ok := enumMessageType[messageType]; !ok {
			return resp, fmt.Errorf("unknown message type: 0x%x", messageType)
		}
		return resp, fmt.Errorf("unsupported message type: 0x%x", messageType)
	}

	return resp, nil
}

func (t *TTSWsClient) connect() (*websocket.Conn, error) {
	u := url.URL{
		Scheme: "wss",
		Host:   "openspeech.bytedance.com",
		Path:   "/api/v1/tts/ws_binary",
	}
	header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer;%s", t.apptoken)}}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return nil, fmt.Errorf("websocket connection failed: %v", err)
	}
	return conn, nil
}

func (t *TTSWsClient) buildRequest(input []byte) ([]byte, error) {
	compressed, err := gzipCompress(input)
	if err != nil {
		return nil, fmt.Errorf("request compression failed: %v", err)
	}

	payloadSize := len(compressed)
	payloadArr := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadArr, uint32(payloadSize))

	request := make([]byte, len(defaultHeader))
	copy(request, defaultHeader)
	request = append(request, payloadArr...)
	request = append(request, compressed...)

	return request, nil
}

// NonStreamSynth 执行一次性语音合成
func (t *TTSWsClient) NonStreamSynth(text, voiceType, outFile string) error {
	input, err := t.SetupInput(text, voiceType, optQuery)
	if err != nil {
		return fmt.Errorf("request setup failed: %v", err)
	}
	fmt.Printf("Request payload: %s\n", string(input))

	request, err := t.buildRequest(input)
	if err != nil {
		return err
	}

	conn, err := t.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		return fmt.Errorf("write request failed: %v", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read response failed: %v", err)
	}

	resp, err := t.parseResponse(message)
	if err != nil {
		return fmt.Errorf("parse response failed: %v", err)
	}

	if err := os.WriteFile(outFile, resp.Audio, 0644); err != nil {
		return fmt.Errorf("write output file failed: %v", err)
	}

	return nil
}

// StreamSynth 执行流式语音合成
func (t *TTSWsClient) StreamSynth(text, voiceType, outFile string) error {
	input, err := t.SetupInput(text, voiceType, optSubmit)
	if err != nil {
		return fmt.Errorf("request setup failed: %v", err)
	}
	fmt.Printf("Request payload: %s\n", string(input))

	request, err := t.buildRequest(input)
	if err != nil {
		return err
	}

	conn, err := t.connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, request); err != nil {
		return fmt.Errorf("write request failed: %v", err)
	}

	var audio []byte
	var lastErr error

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			lastErr = fmt.Errorf("read message failed: %v", err)
			break
		}

		resp, err := t.parseResponse(message)
		if err != nil {
			lastErr = fmt.Errorf("parse response failed: %v", err)
			break
		}

		audio = append(audio, resp.Audio...)
		if resp.IsLast {
			break
		}
	}

	if len(audio) > 0 {
		if err := os.WriteFile(outFile, audio, 0644); err != nil {
			return fmt.Errorf("write output file failed: %v", err)
		}
	}

	if lastErr != nil {
		return fmt.Errorf("stream synthesis completed with error: %v", lastErr)
	}

	return nil
}
