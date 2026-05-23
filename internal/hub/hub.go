package hub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cjhargre/mainstacks/internal/skill"
)

const (
	repo    = "cjhargreaves/mainstacks-hub"
	baseURL = "https://api.github.com"
)

type Client struct {
	token string
}

func New(token string) *Client {
	return &Client{token: token}
}

type CommunitySkill struct {
	skill.Skill
	Author string `json:"author"`
}

// Publish uploads a skill to the hub. Requires GITHUB_TOKEN.
func (c *Client) Publish(sk skill.Skill, author string) error {
	if c.token == "" {
		return fmt.Errorf("GITHUB_TOKEN required to publish. Set it in your environment")
	}
	cs := CommunitySkill{Skill: sk, Author: author}
	data, _ := json.MarshalIndent(cs, "", "  ")

	filename := strings.ReplaceAll(strings.ToLower(sk.Name), " ", "-") + ".json"
	path := "skills/" + filename

	body := map[string]string{
		"message": "publish: " + sk.Name,
		"content": base64.StdEncoding.EncodeToString(data),
	}
	bodyJSON, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", fmt.Sprintf("%s/repos/%s/contents/%s", baseURL, repo, path), bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: %s %s", resp.Status, string(b))
	}
	return nil
}

// Browse lists all community skills. No token needed (public repo).
func (c *Client) Browse() ([]CommunitySkill, error) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s/contents/skills", baseURL, repo), nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // no skills folder yet
	}

	var files []struct {
		Path string `json:"path"`
	}
	json.NewDecoder(resp.Body).Decode(&files)

	var skills []CommunitySkill
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".json") {
			continue
		}
		sk, err := c.fetchSkill(f.Path)
		if err == nil {
			skills = append(skills, sk)
		}
	}
	return skills, nil
}

// Search finds community skills matching a query by filtering locally.
func (c *Client) Search(query string) ([]CommunitySkill, error) {
	all, err := c.Browse()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var results []CommunitySkill
	for _, sk := range all {
		if strings.Contains(strings.ToLower(sk.Name), q) ||
			strings.Contains(strings.ToLower(sk.Summary), q) ||
			strings.Contains(strings.ToLower(strings.Join(sk.Tags, " ")), q) {
			results = append(results, sk)
		}
	}
	return results, nil
}

func (c *Client) fetchSkill(path string) (CommunitySkill, error) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/repos/%s/contents/%s", baseURL, repo, path), nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return CommunitySkill{}, err
	}
	defer resp.Body.Close()

	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	json.NewDecoder(resp.Body).Decode(&file)

	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil {
		return CommunitySkill{}, err
	}

	var sk CommunitySkill
	err = json.Unmarshal(decoded, &sk)
	return sk, err
}
