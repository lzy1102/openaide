package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// VoiceService 语音交互服务
type VoiceService struct {
	config     VoiceConfig
	httpClient *http.Client
}

// VoiceConfig 语音服务配置
type VoiceConfig struct {
	Enabled     bool   `json:"enabled"`
	WhisperAPI  string `json:"whisper_api"`  // STT API endpoint (OpenAI Whisper compatible)
	WhisperKey  string `json:"whisper_key"`  // STT API key
	TTSAPI      string `json:"tts_api"`      // TTS API endpoint
	TTSKey      string `json:"tts_key"`      // TTS API key
	TTSVoice    string `json:"tts_voice"`    // TTS voice name
	DefaultLang string `json:"default_lang"` // 默认语言
}

// STTResult 语音识别结果
type STTResult struct {
	Text      string  `json:"text"`
	Language  string  `json:"language"`
	Duration  float64 `json:"duration"`
	Confidence float64 `json:"confidence"`
}

// TTSResult 语音合成结果
type TTSResult struct {
	AudioData []byte `json:"-"`
	Format    string `json:"format"`
	Duration  float64 `json:"duration"`
}

// NewVoiceService 创建语音服务
func NewVoiceService(config VoiceConfig) *VoiceService {
	return &VoiceService{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// IsEnabled 检查语音服务是否可用
func (s *VoiceService) IsEnabled() bool {
	return s.config.Enabled
}

// SpeechToText 语音转文字
// audioData: 音频二进制数据
// format: 音频格式 (wav, mp3, ogg, flac, webm)
func (s *VoiceService) SpeechToText(ctx context.Context, audioData []byte, format string) (*STTResult, error) {
	if !s.config.Enabled || s.config.WhisperAPI == "" {
		return nil, fmt.Errorf("voice service not enabled or STT API not configured")
	}

	startTime := time.Now()

	// 构建 multipart 请求
	body := &bytes.Buffer{}
	boundary := "voice-boundary-" + fmt.Sprintf("%d", time.Now().UnixNano())

	_, err := fmt.Fprintf(body, "--%s\r\n", boundary)
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprintf(body, "Content-Disposition: form-data; name=\"file\"; filename=\"audio.%s\"\r\n", format)
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprintf(body, "Content-Type: audio/%s\r\n\r\n", format)
	if err != nil {
		return nil, err
	}
	_, err = body.Write(audioData)
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprintf(body, "\r\n--%s\r\n", boundary)
	if err != nil {
		return nil, err
	}
	_, err = fmt.Fprintf(body, "Content-Disposition: form-data; name=\"model\"\r\n\r\nwhisper-1\r\n")
	if err != nil {
		return nil, err
	}

	if s.config.DefaultLang != "" {
		_, err = fmt.Fprintf(body, "\r\n--%s\r\n", boundary)
		if err != nil {
			return nil, err
		}
		_, err = fmt.Fprintf(body, "Content-Disposition: form-data; name=\"language\"\r\n\r\n%s\r\n", s.config.DefaultLang)
		if err != nil {
			return nil, err
		}
	}
	_, err = fmt.Fprintf(body, "--%s--\r\n", boundary)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WhisperAPI, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	if s.config.WhisperKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.WhisperKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("STT API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read STT response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("STT API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Duration float64 `json:"duration"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse STT response: %w", err)
	}

	log.Printf("[Voice] STT completed in %v, language=%s", time.Since(startTime), result.Language)

	return &STTResult{
		Text:      result.Text,
		Language:  result.Language,
		Duration:  result.Duration,
	}, nil
}

// SpeechToTextBase64 语音转文字（Base64 输入）
func (s *VoiceService) SpeechToTextBase64(ctx context.Context, audioBase64 string, format string) (*STTResult, error) {
	audioData, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 audio data: %w", err)
	}
	return s.SpeechToText(ctx, audioData, format)
}

// TextToSpeech 文字转语音
func (s *VoiceService) TextToSpeech(ctx context.Context, text string) (*TTSResult, error) {
	if !s.config.Enabled || s.config.TTSAPI == "" {
		return nil, fmt.Errorf("voice service not enabled or TTS API not configured")
	}

	startTime := time.Now()

	reqBody := map[string]interface{}{
		"input": text,
	}
	if s.config.TTSVoice != "" {
		reqBody["voice"] = s.config.TTSVoice
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.TTSAPI, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.config.TTSKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.TTSKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TTS API call failed: %w", err)
	}
	defer resp.Body.Close()

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read TTS response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TTS API returned %d: %s", resp.StatusCode, string(audioData))
	}

	contentType := resp.Header.Get("Content-Type")
	format := "mp3"
	if contentType != "" {
		switch {
		case contentTypeContains(contentType, "ogg"):
			format = "ogg"
		case contentTypeContains(contentType, "wav"):
			format = "wav"
		case contentTypeContains(contentType, "mp3"):
			format = "mp3"
		case contentTypeContains(contentType, "webm"):
			format = "webm"
		}
	}

	log.Printf("[Voice] TTS completed in %v, format=%s, size=%d", time.Since(startTime), format, len(audioData))

	return &TTSResult{
		AudioData: audioData,
		Format:    format,
		Duration:  float64(time.Since(startTime).Seconds()),
	}, nil
}

// TextToSpeechBase64 文字转语音（Base64 输出）
func (s *VoiceService) TextToSpeechBase64(ctx context.Context, text string) (string, string, error) {
	result, err := s.TextToSpeech(ctx, text)
	if err != nil {
		return "", "", err
	}
	encoded := base64.StdEncoding.EncodeToString(result.AudioData)
	return encoded, result.Format, nil
}

// GetStatus 获取语音服务状态
func (s *VoiceService) GetStatus() map[string]interface{} {
	status := map[string]interface{}{
		"enabled":    s.config.Enabled,
		"stt_ready":  s.config.Enabled && s.config.WhisperAPI != "",
		"tts_ready":  s.config.Enabled && s.config.TTSAPI != "",
		"voice":      s.config.TTSVoice,
		"lang":       s.config.DefaultLang,
	}
	return status
}

func contentTypeContains(contentType, substr string) bool {
	for i := 0; i <= len(contentType)-len(substr); i++ {
		if contentType[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
