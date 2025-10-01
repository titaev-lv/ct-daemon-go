package exchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CexRestClient — базовый клиент для REST-запросов к CEX

type CexRestClient struct {
	BaseURL string
	Client  *http.Client
}

func NewCexRestClient(baseURL string) *CexRestClient {
	return &CexRestClient{
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetJSON выполняет GET-запрос и декодирует JSON-ответ
func (c *CexRestClient) GetJSON(path string, result interface{}) error {
	url := c.BaseURL + path
	resp, err := c.Client.Get(url)
	if err != nil {
		return fmt.Errorf("CexRestClient GET error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("CexRestClient GET status: %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// PostJSON выполняет POST-запрос с JSON и декодирует JSON-ответ
func (c *CexRestClient) PostJSON(path string, payload, result interface{}) error {
	url := c.BaseURL + path
	fmt.Printf("[DEBUG] CexRestClient POST URL: %s\n", url)

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("[DEBUG] CexRestClient marshal error: %v\n", err)
		return fmt.Errorf("CexRestClient marshal error: %w", err)
	}
	fmt.Printf("[DEBUG] CexRestClient POST body: %s\n", string(body))

	resp, err := c.Client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("[DEBUG] CexRestClient POST network error: %v\n", err)
		return fmt.Errorf("CexRestClient POST error: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("[DEBUG] CexRestClient POST response status: %d\n", resp.StatusCode)

	if resp.StatusCode != 200 {
		// Читаем тело ответа для debug
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("[DEBUG] CexRestClient POST error response body: %s\n", string(bodyBytes))
		return fmt.Errorf("CexRestClient POST status: %d", resp.StatusCode)
	}

	// Читаем ответ для логирования
	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("[DEBUG] CexRestClient error reading response: %v\n", err)
		return fmt.Errorf("CexRestClient read error: %w", err)
	}
	fmt.Printf("[DEBUG] CexRestClient POST response body: %s\n", string(respBodyBytes))

	// Декодируем из прочитанных байтов
	return json.Unmarshal(respBodyBytes, result)
}

// DoRequest — универсальный метод для любых http.Request
func (c *CexRestClient) DoRequest(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// LogResponseBody логирует тело ответа
func (c *CexRestClient) LogResponseBody(resp *http.Response) {
	if resp.Body == nil {
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Ошибка чтения тела ответа: %v\n", err)
		return
	}

	fmt.Printf("Тело ответа: %s\n", body)
}
