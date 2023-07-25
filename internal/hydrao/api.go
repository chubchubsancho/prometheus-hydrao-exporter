package hydrao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	// DefaultBaseURL is Hydrao api url
	baseURL = "https://api.hydrao.com/"
	// DefaultAuthURL is Hydrao auth url
	authURL = baseURL + "sessions"
	// DefaultShowerHeadsURL is Hydrao showerheads url
	showerheadsURL = baseURL + "shower-heads/"
)

// Config is used to specify credential to Hydrao API
// Email : Your hydrao account email
// Password : Your hydrao account password
// ApiKey : Your hydrao account api-key
type Config struct {
	Email    string
	Password string
	ApiKey   string
}

type SessionInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int16  `json:"expires_in"`
	TokenRefresh time.Time
	Config       Config
}

type Client struct {
	httpClient   http.Client
	ctx          context.Context
	logger       log.Logger
	httpResponse *http.Response
	hydrao       *SessionInfo
}

// NewClient create a handle authentication to Hydrao API
func NewClient(config Config, logger log.Logger) *Client {

	return &Client{
		httpClient: http.Client{
			Transport: &http.Transport{
				//TLSClientConfig:     newTLSConfig(),
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     10 * time.Minute,
			},
			Timeout: 10 * time.Second,
		},
		ctx:    context.Background(),
		logger: logger,
		hydrao: &SessionInfo{
			AccessToken:  "",
			RefreshToken: "",
			ExpiresIn:    0,
			Config: Config{
				Email:    config.Email,
				Password: config.Password,
				ApiKey:   config.ApiKey,
			},
		},
	}
}

func (c *Client) NewSession() error {
	buffer := new(bytes.Buffer)
	data := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{
		Email:    c.hydrao.Config.Email,
		Password: c.hydrao.Config.Password,
	}
	if err := json.NewEncoder(buffer).Encode(data); err != nil {
		return err
	}
	now := time.Now()

	resp, err := c.doHTTPPost(authURL, c.hydrao.Config, buffer)
	if err != nil {
		return err
	}

	if err = processHTTPResponse(resp, err, c.hydrao); err != nil {
		return err
	}
	c.hydrao.TokenRefresh = now

	return nil
}

func (c *Client) RefreshSession() error {
	buffer := new(bytes.Buffer)
	data := struct {
		RefreshToken string `json:"refresh_token"`
	}{
		RefreshToken: c.hydrao.RefreshToken,
	}
	if err := json.NewEncoder(buffer).Encode(data); err != nil {
		return err
	}
	now := time.Now()

	resp, err := c.doHTTPPost(authURL, c.hydrao.Config, buffer)
	if err != nil {
		return err
	}

	if err = processHTTPResponse(resp, err, c.hydrao); err != nil {
		return err
	}
	c.hydrao.TokenRefresh = now

	return nil
}


// do a url encoded HTTP POST request
func (c *Client) doHTTPPost(url string, config Config, data io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, data)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", config.ApiKey)
	// for _, cb := range callbacks {
	// 	cb(req)
	// }
	return c.do(req)
}

// do a generic HTTP request
func (c *Client) do(req *http.Request) (*http.Response, error) {

	//debug
	//remove or comment before build
	// debug, _ := httputil.DumpRequestOut(req, true)
	// fmt.Printf("%s\n\n", debug)

	var err error
	c.httpResponse, err = c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return c.httpResponse, nil
}

// process HTTP response
// Unmarshall received data into holder struct
func processHTTPResponse(resp *http.Response, err error, holder interface{}) error {
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	//debug
	//remove or comment before build
	// debug, _ := httputil.DumpResponse(resp, true)
	// fmt.Printf("%s\n\n", debug)

	// check http return code
	if resp.StatusCode != 200 {
		//bytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("bad http return code %d", resp.StatusCode)
	}

	// Unmarshall response into given struct
	if err = json.NewDecoder(resp.Body).Decode(&holder); err != nil {
		return err
	}

	return nil
}
