package horizon

import (
	"encoding/json"
	"gitlab.com/distributed_lab/logan/v3/errors"
	"net/http"
	"path"
)

type horizon struct {
	Client *http.Client
	URL    string `fig:"url"`
}

func (h *horizon) do(method, endpoint string, data interface{}) error {
	req, err := http.NewRequest(method, path.Join(h.URL, endpoint), nil)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	resp, err := h.Client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return errors.Wrap(err, "failed to do request")
	}

	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(data)
}
