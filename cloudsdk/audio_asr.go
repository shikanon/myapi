package cloudsdk

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	PROTOCOL_VERSION    = 0x01
	DEFAULT_HEADER_SIZE = 0x01

	// Message Types
	FULL_CLIENT_REQUEST   = 0x01
	AUDIO_ONLY_REQUEST    = 0x02
	FULL_SERVER_RESPONSE  = 0x09
	SERVER_ACK            = 0x0B
	SERVER_ERROR_RESPONSE = 0x0F

	// Message Type Specific Flags
	NO_SEQUENCE       = 0x00
	POS_SEQUENCE      = 0x01
	NEG_SEQUENCE      = 0x02
	NEG_WITH_SEQUENCE = 0x03

	// Serialization Methods
	NO_SERIALIZATION = 0x00
	JSON             = 0x01

	// Compression Types
	NO_COMPRESSION   = 0x00
	GZIP_COMPRESSION = 0x01
)

type AsrConfig struct {
	SegDuration int
	WsURL       string
	UID         string
	Format      string
	Rate        int
	Bits        int
	Channel     int
	Codec       string
	AuthMethod  string
	HotWords    string
	Streaming   bool
	Mp3SegSize  int
	AccessKey   string // Add this
	AppKey      string // Add this
}

type AsrWsClient struct {
	config *AsrConfig
}

type Request struct {
	User struct {
		UID string `json:"uid"`
	} `json:"user"`
	Audio struct {
		Format     string `json:"format"`
		SampleRate int    `json:"sample_rate"`
		Bits       int    `json:"bits"`
		Channel    int    `json:"channel"`
		Codec      string `json:"codec"`
	} `json:"audio"`
	Request struct {
		ModelName  string `json:"model_name"`
		EnablePunc bool   `json:"enable_punc"`
	} `json:"request"`
}

type Response struct {
	IsLastPackage   bool
	PayloadSequence int
	Seq             int
	Code            int
	PayloadMsg      interface{}
	PayloadSize     int
}

func NewAsrWsClient(config *AsrConfig) *AsrWsClient {
	return &AsrWsClient{config: config}
}

func generateHeader(messageType, messageTypeSpecificFlags, serialMethod, compressionType, reservedData byte) []byte {
	header := make([]byte, 4)
	header[0] = (PROTOCOL_VERSION << 4) | DEFAULT_HEADER_SIZE
	header[1] = (messageType << 4) | messageTypeSpecificFlags
	header[2] = (serialMethod << 4) | compressionType
	header[3] = reservedData
	return header
}

func generateBeforePayload(sequence int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(sequence))
	return buf
}

func parseResponse(res []byte) *Response {
	result := &Response{}

	if len(res) < 4 {
		return result
	}

	headerSize := res[0] & 0x0F
	messageType := res[1] >> 4
	messageTypeSpecificFlags := res[1] & 0x0F
	serializationMethod := res[2] >> 4
	messageCompression := res[2] & 0x0F
	_ = res[3] // reserved

	payload := res[headerSize*4:]

	if messageTypeSpecificFlags&0x01 != 0 {
		// receive frame with sequence
		seq := binary.BigEndian.Uint32(payload[:4])
		result.PayloadSequence = int(seq)
		payload = payload[4:]
	}

	if messageTypeSpecificFlags&0x02 != 0 {
		// receive last package
		result.IsLastPackage = true
	}

	var payloadMsg []byte
	var payloadSize int

	switch messageType {
	case FULL_SERVER_RESPONSE:
		payloadSize = int(binary.BigEndian.Uint32(payload[:4]))
		payloadMsg = payload[4:]
	case SERVER_ACK:
		seq := binary.BigEndian.Uint32(payload[:4])
		result.Seq = int(seq)
		if len(payload) >= 8 {
			payloadSize = int(binary.BigEndian.Uint32(payload[4:8]))
			payloadMsg = payload[8:]
		}
	case SERVER_ERROR_RESPONSE:
		code := binary.BigEndian.Uint32(payload[:4])
		result.Code = int(code)
		if len(payload) >= 8 {
			payloadSize = int(binary.BigEndian.Uint32(payload[4:8]))
			payloadMsg = payload[8:]
		}
	}

	if payloadMsg != nil {
		if messageCompression == GZIP_COMPRESSION {
			gr, err := gzip.NewReader(bytes.NewReader(payloadMsg))
			if err == nil {
				decompressed, err := io.ReadAll(gr)
				if err == nil {
					payloadMsg = decompressed
				}
			}
		}

		if serializationMethod == JSON {
			var msg interface{}
			if err := json.Unmarshal(payloadMsg, &msg); err == nil {
				result.PayloadMsg = msg
			}
		} else if serializationMethod != NO_SERIALIZATION {
			result.PayloadMsg = string(payloadMsg)
		}

		result.PayloadSize = payloadSize
	}

	return result
}

func (c *AsrWsClient) constructRequest(reqID string) *Request {
	req := &Request{}
	req.User.UID = c.config.UID
	req.Audio.Format = c.config.Format
	req.Audio.SampleRate = c.config.Rate
	req.Audio.Bits = c.config.Bits
	req.Audio.Channel = c.config.Channel
	req.Audio.Codec = c.config.Codec
	req.Request.ModelName = "bigmodel"
	req.Request.EnablePunc = true
	return req
}

func (c *AsrWsClient) sliceData(data []byte, chunkSize int) <-chan struct {
	Chunk []byte
	Last  bool
} {
	ch := make(chan struct {
		Chunk []byte
		Last  bool
	})

	go func() {
		defer close(ch)
		dataLen := len(data)
		offset := 0

		for offset+chunkSize < dataLen {
			ch <- struct {
				Chunk []byte
				Last  bool
			}{
				Chunk: data[offset : offset+chunkSize],
				Last:  false,
			}
			offset += chunkSize
		}

		ch <- struct {
			Chunk []byte
			Last  bool
		}{
			Chunk: data[offset:dataLen],
			Last:  true,
		}
	}()

	return ch
}

func (c *AsrWsClient) RecognizeStream(audioPath string) (*Response, error) {
	data, err := os.ReadFile(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %v", err)
	}

	var segmentSize int
	switch c.config.Format {
	case "mp3":
		segmentSize = c.config.Mp3SegSize
	case "wav":
		// Simplified - in real implementation you'd parse WAV header
		sizePerSec := c.config.Channel * (c.config.Bits / 8) * c.config.Rate
		segmentSize = int(sizePerSec * c.config.SegDuration / 1000)
	case "pcm":
		segmentSize = int(c.config.Rate * 2 * c.config.Channel * c.config.SegDuration / 500)
	default:
		return nil, fmt.Errorf("unsupported format: %s", c.config.Format)
	}

	return c.processData(data, segmentSize)
}

func (c *AsrWsClient) processData(data []byte, segmentSize int) (*Response, error) {
	reqID := uuid.New().String()
	seq := 1

	requestParams := c.constructRequest(reqID)
	payloadBytes, err := json.Marshal(requestParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payloadBytes); err != nil {
		return nil, fmt.Errorf("failed to compress payload: %v", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %v", err)
	}
	compressedPayload := buf.Bytes()

	fullClientRequest := generateHeader(FULL_CLIENT_REQUEST, POS_SEQUENCE, JSON, GZIP_COMPRESSION, 0x00)
	fullClientRequest = append(fullClientRequest, generateBeforePayload(seq)...)
	fullClientRequest = append(fullClientRequest, make([]byte, 4)...)
	binary.BigEndian.PutUint32(fullClientRequest[len(fullClientRequest)-4:], uint32(len(compressedPayload)))
	fullClientRequest = append(fullClientRequest, compressedPayload...)

	headers := make(map[string][]string)
	headers["X-Api-Resource-Id"] = []string{"volc.bigasr.sauc.duration"}
	headers["X-Api-Access-Key"] = []string{c.config.AccessKey}
	headers["X-Api-App-Key"] = []string{c.config.AppKey}
	headers["X-Api-Request-Id"] = []string{reqID}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(c.config.WsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, fullClientRequest); err != nil {
		return nil, fmt.Errorf("failed to send initial request: %v", err)
	}

	_, res, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read initial response: %v", err)
	}

	result := parseResponse(res)
	log.Printf("Initial response: %+v", result)

	for chunkData := range c.sliceData(data, segmentSize) {
		seq++
		if chunkData.Last {
			seq = -seq
		}

		start := time.Now()

		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(chunkData.Chunk); err != nil {
			return nil, fmt.Errorf("failed to compress chunk: %v", err)
		}
		if err := gz.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip writer: %v", err)
		}
		compressedChunk := buf.Bytes()

		var audioOnlyRequest []byte
		if chunkData.Last {
			audioOnlyRequest = generateHeader(AUDIO_ONLY_REQUEST, NEG_WITH_SEQUENCE, JSON, GZIP_COMPRESSION, 0x00)
		} else {
			audioOnlyRequest = generateHeader(AUDIO_ONLY_REQUEST, POS_SEQUENCE, JSON, GZIP_COMPRESSION, 0x00)
		}

		audioOnlyRequest = append(audioOnlyRequest, generateBeforePayload(seq)...)
		audioOnlyRequest = append(audioOnlyRequest, make([]byte, 4)...)
		binary.BigEndian.PutUint32(audioOnlyRequest[len(audioOnlyRequest)-4:], uint32(len(compressedChunk)))
		audioOnlyRequest = append(audioOnlyRequest, compressedChunk...)

		if err := conn.WriteMessage(websocket.BinaryMessage, audioOnlyRequest); err != nil {
			return nil, fmt.Errorf("failed to send audio chunk: %v", err)
		}

		_, res, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %v", err)
		}

		result = parseResponse(res)
		log.Printf("Response for seq %d: %+v", seq, result)

		if c.config.Streaming {
			sleepTime := time.Duration(c.config.SegDuration)*time.Millisecond - time.Since(start)
			if sleepTime > 0 {
				time.Sleep(sleepTime)
			}
		}
	}

	return result, nil
}
