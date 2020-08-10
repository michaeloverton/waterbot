package esp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// Client is the client for interacting with the notifications API.
type espClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// Client is used to find weddings by memberID.
type Client interface {
	Water(ctx context.Context) error
	Blink(ctx context.Context) error
}

// NewClient returns a new instance of Client.
// httpClient will default to http.DefaultClient if not specified.
func NewClient(url string, httpClient *http.Client) Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 5 * time.Second,
		}
	}

	return &espClient{
		BaseURL:    url,
		HTTPClient: httpClient,
	}
}

func (ec *espClient) Water(ctx context.Context) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/water", ec.BaseURL), nil)
	if err != nil {
		return errors.Wrap(err, "failed to create water request")
	}

	res, err := ec.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "water request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("non-200 status: %d", res.StatusCode)
	}

	return nil
}

func (ec *espClient) Blink(ctx context.Context) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/blink", ec.BaseURL), nil)
	if err != nil {
		return errors.Wrap(err, "failed to create blink request")
	}

	res, err := ec.HTTPClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "blink request failed")
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("non-200 status: %d", res.StatusCode)
	}

	return nil
}
