package mcaccutils

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pmylund/go-cache"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrPlayerNotFound is an error returned when no player is found for the
	// specified query.
	ErrPlayerNotFound = errors.New("mcaccutils: player not found")

	// CacheDuration can be used to modify the duration fetched names and UUIDs
	// are cached for. Making this duration very short can make it much easier
	// to go over the Mojang rate limits, so it is not recommended.
	CacheDuration = 12 * time.Hour

	// dataCache is the memory cache for all names. The default expiration time
	// means nothing, because CacheDuration is used in all cases when values are
	// added to the cache.
	dataCache = cache.New(1*time.Hour, 1*time.Minute)
)

type playerCacheData struct {
	UUID     string
	Username string
}

// GetNames produces a list of all usernames ever owned by the specified UUID, in
// unspecified order.
//
// The result of this function is not cached, so it should be used with caution
// so as to avoid running into the Mojang rate limit.
func GetNames(uuid string) (names []string, err error) {
	uuid = strings.Replace(uuid, "-", "", -1)
	// Fetch the account info API for this player UUID.
	resp, err := http.Get(fmt.Sprintf("https://api.mojang.com/user/profiles/%s/names", uuid))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Read out the body.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Decode the JSON
	var decResp []string
	err = json.Unmarshal(body, &decResp)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, ErrPlayerNotFound
	}
	// Return the decoded names.
	return decResp, nil
}

// GetName returns the first name found by the Mojang API for the specified
// UUID, or an error if the name cannot be found.
func GetName(uuid string) (name string, err error) {
	uuid = strings.Replace(uuid, "-", "", -1)
	if p, found := dataCache.Get(uuid); found {
		return p.(*playerCacheData).Username, nil
	}
	names, err := GetNames(uuid)
	if err != nil {
		return "", err
	}
	p := &playerCacheData{UUID: uuid, Username: names[0]}
	dataCache.Add(strings.ToLower(names[0]), p, CacheDuration)
	dataCache.Add(uuid, p, CacheDuration)
	return names[0], nil
}

type mojangNameResponse struct {
	Profiles []mojangNameResponseProfile `json:"profiles"`
	Count    int                         `json:"size"`
}

type mojangNameResponseProfile struct {
	Name string `json:"name"`
	UUID string `json:"id"`
}

// GetUUID takes the player name and returns the UUID of that player, and the
// case corrected username. It returns a UUID which does not contain dashes (-).
func GetUUID(n string) (uuid string, name string, err error) {
	n = strings.ToLower(n)
	// Try the cache.
	p, found := dataCache.Get(n)
	if found {
		return p.(*playerCacheData).UUID, p.(*playerCacheData).Username, nil
	}
	// Hit the API and wait for a response.
	reqBody := strings.NewReader(
		fmt.Sprintf("{\"name\":\"%s\", \"agent\": \"minecraft\"}", n),
	)
	resp, err := http.Post("https://api.mojang.com/profiles/page/1", "application/json", reqBody)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	// Read out the body.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	// Decode the JSON
	decResp := mojangNameResponse{}
	err = json.Unmarshal(body, &decResp)
	if err != nil {
		return "", "", err
	}
	// Make sure the lookup was a success.
	if decResp.Count < 1 {
		return "", "", ErrPlayerNotFound
	}
	u := strings.Replace(decResp.Profiles[0].UUID, "-", "", -1)
	p = &playerCacheData{UUID: u, Username: n}
	dataCache.Add(n, p, CacheDuration)
	dataCache.Add(u, p, CacheDuration)
	return u, decResp.Profiles[0].Name, nil
}
