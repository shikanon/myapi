package cloudsdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type AIChatAPI struct {
	apiKey        string
	endpointID32k string
	url           string
	headers       map[string]string
	TotalTokens   float64
	tokenMutex    sync.Mutex
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Payload struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

func (api *AIChatAPI) Init(apiKey, endpointID32k string) {
	api.apiKey = apiKey
	api.endpointID32k = endpointID32k
	api.url = "https://ark.cn-beijing.volces.com/api/v3/chat/completions"
	api.headers = map[string]string{
		"Content-Type":  "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", api.apiKey),
	}
}

// SendMessageAsync 异步发送消息
func (api *AIChatAPI) SendMessageAsync(systemPrompt, userMessage string, resultChan chan<- string, errorChan chan<- error) {
	go func() {
		msgContent := fmt.Sprintf(">>>输入>>>\n%s\n>>>输出>>>\n", userMessage)
		payload := Payload{
			Model: api.endpointID32k,
			Messages: []Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: msgContent},
			},
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			errorChan <- fmt.Errorf("JSON 编码失败: %v", err)
			return
		}

		client := http.Client{Timeout: time.Hour}
		req, err := http.NewRequest("POST", api.url, bytes.NewBuffer(jsonData))
		if err != nil {
			errorChan <- fmt.Errorf("创建 HTTP 请求失败: %v", err)
			return
		}

		for key, value := range api.headers {
			req.Header.Set(key, value)
		}

		resp, err := client.Do(req)
		if err != nil {
			errorChan <- fmt.Errorf("发送请求失败: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errorChan <- fmt.Errorf("API 请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
			return
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			errorChan <- fmt.Errorf("读取响应数据失败: %v", err)
			return
		}

		var responseMap map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &responseMap); err != nil {
			errorChan <- fmt.Errorf("JSON 解码失败: %v", err)
			return
		}

		// 加锁更新 TotalTokens
		api.tokenMutex.Lock()
		api.TotalTokens = api.TotalTokens + responseMap["usage"].(map[string]interface{})["total_tokens"].(float64)
		api.tokenMutex.Unlock()

		result := responseMap["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"].(string)
		resultChan <- result
	}()
}

// WorkflowRequest 定义请求参数的结构体
type Workflow struct {
	Token      string
	WorkflowID string
}

type WorkflowRequest struct {
	WorkflowID string                 `json:"workflow_id"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Ext        map[string]string      `json:"ext,omitempty"`
	IsAsync    bool                   `json:"is_async"`
}

// WorkflowResponse 定义响应参数的结构体
type WorkflowResponse struct {
	Code      int             `json:"code"`
	Msg       string          `json:"msg"`
	Data      json.RawMessage `json:"data"`
	ExecuteID string          `json:"execute_id"`
	DebugURL  string          `json:"debug_url"`
	Token     int             `json:"token"`
	Cost      string          `json:"cost"`
}

// runWorkflow 封装调用工作流接口的函数
func (w *Workflow) RunWorkflow(request WorkflowRequest) (*WorkflowResponse, error) {
	apiURL := "https://api.coze.cn/v1/workflow/run"
	// 序列化请求体
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	// 添加HTTP头
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", w.Token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")

	// 发送HTTP请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 反序列化响应体到结构体
	var workflowResponse WorkflowResponse
	err = json.Unmarshal(body, &workflowResponse)
	if err != nil {
		return nil, err
	}

	return &workflowResponse, nil
}
