package remote

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ygpkg/yg-go/apis/apiobj"
	"github.com/ygpkg/yg-go/encryptor"
	"gopkg.in/yaml.v3"
)

// RemoteSettingClient is a client to get remote settings from a remote server.
type RemoteSettingClient struct {
	BaseURL string
	AK, SK  string
	Group   string
	cli     *http.Client
}

// NewRemoteSettingClient creates a new RemoteSettingClient.
func NewRemoteSettingClient(baseURL, ak, sk, group string) *RemoteSettingClient {
	return &RemoteSettingClient{
		BaseURL: baseURL,
		AK:      ak,
		SK:      sk,
		Group:   group,
		cli:     http.DefaultClient,
	}
}

// NewRemoteSettingClientWithEnv creates a new RemoteSettingClient with env.
func NewRemoteSettingClientWithEnv() *RemoteSettingClient {
	return NewRemoteSettingClient(
		"https://yygu.cn/v2/cook.GetSettingContent",
		os.Getenv("YGCFG_AK"),
		os.Getenv("YGCFG_SK"),
		os.Getenv("YGCFG_GROUP"),
	)
}

// getSettingContentRequest 获取配置内容
type getSettingContentRequest struct {
	apiobj.BaseRequest
	Request struct {
		GroupName  string `json:"group"`
		SettingKey string `json:"setting_key"`
	}
}

// getSettingContentResponse 获取配置内容
type getSettingContentResponse struct {
	apiobj.BaseResponse
	Response struct {
		Content string `json:"content,omitempty"`
		Type    string `json:"type,omitempty"`
		Nonce   string `json:"nonce,omitempty"`
	}
}

// Get gets the remote settings.
func (c *RemoteSettingClient) Get(key string) (string, error) {
	in := getSettingContentRequest{}
	in.Request.GroupName = c.Group
	in.Request.SettingKey = key
	out := getSettingContentResponse{}
	err := c.doReuqest(&in, &out, "")
	if err != nil {
		return "", err
	}
	if out.Code == 0 {
		return out.Response.Content, nil
	}
	if out.Code != http.StatusUnauthorized {
		return "", fmt.Errorf("remote setting error: %s", out.Message)
	}
	nonce := out.Response.Nonce
	tokenSli := []string{c.AK, nonce}

	expectedSignature := encryptor.HmacHash(sha1.New, c.SK, strings.Join(tokenSli, "."))
	tokenSli = append(tokenSli, expectedSignature)
	token := strings.Join(tokenSli, ".")
	err = c.doReuqest(&in, &out, token)
	if err != nil {
		return "", err
	}
	if out.Code != 0 {
		return "", fmt.Errorf("remote setting error: %s", out.Message)
	}
	return out.Response.Content, nil
}

func (c *RemoteSettingClient) doReuqest(in *getSettingContentRequest, out *getSettingContentResponse, token string) error {
	inBody := new(bytes.Buffer)
	json.NewEncoder(inBody).Encode(in)
	req, err := http.NewRequest("POST", c.BaseURL, inBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := c.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&out)
	return nil
}

var stdRemoteClient = NewRemoteSettingClientWithEnv()

func InitRemoteSettingClientWithEnv(cli *RemoteSettingClient) {
	stdRemoteClient = cli
}

func GetRemoteText(key string) (string, error) {
	return stdRemoteClient.Get(key)
}

func GetRemoteYAML(key string, val interface{}) error {
	content, err := stdRemoteClient.Get(key)
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(content), val)
}

func GetRemoteJSON(key string, val interface{}) error {
	content, err := stdRemoteClient.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(content), val)
}
