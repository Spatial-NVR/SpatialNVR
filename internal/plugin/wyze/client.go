// Package wyze provides a Wyze camera plugin for the NVR system.
// It integrates with Wyze cameras via their cloud API and wyze-bridge for streaming.
package wyze

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Wyze API endpoints
const (
	authURL    = "https://auth-prod.api.wyze.com/api/user/login"
	apiBaseURL = "https://api.wyzecam.com"

	// API constants
	appVer   = "wyze_developer_api"
	phoneSC  = "wyze_developer_api"
	phoneID  = "wyze_developer_api"
	appName  = "wyze_developer_api"
)

// Client is the Wyze cloud API client
type Client struct {
	email    string
	password string
	apiKey   string
	keyID    string

	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
	userID       string

	http *http.Client
	mu   sync.RWMutex
}

// NewClient creates a new Wyze API client
func NewClient(email, password, apiKey, keyID string) *Client {
	return &Client{
		email:    email,
		password: password,
		apiKey:   apiKey,
		keyID:    keyID,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Login authenticates with the Wyze cloud
func (c *Client) Login(ctx context.Context) error {
	// Hash the password
	hasher := md5.New()
	hasher.Write([]byte(c.password))
	hasher.Write([]byte(c.password))
	hasher.Write([]byte(c.password))
	passwordHash := hex.EncodeToString(hasher.Sum(nil))

	payload := map[string]interface{}{
		"email":    c.email,
		"password": passwordHash,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", authURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Keyid", c.keyID)
	req.Header.Set("Apikey", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
		ExpiresIn    int    `json:"expires_in"`
		MfaOptions   []string `json:"mfa_options"`
		MfaDetails   interface{} `json:"mfa_details"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse login response: %w", err)
	}

	// Check for MFA requirement
	if len(result.MfaOptions) > 0 {
		return fmt.Errorf("MFA is required for this account - please use wyze-bridge instead")
	}

	if result.AccessToken == "" {
		return fmt.Errorf("login failed: no access token returned")
	}

	c.mu.Lock()
	c.accessToken = result.AccessToken
	c.refreshToken = result.RefreshToken
	c.userID = result.UserID
	if result.ExpiresIn > 0 {
		c.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	} else {
		c.tokenExpiry = time.Now().Add(23 * time.Hour) // Default to 23 hours
	}
	c.mu.Unlock()

	return nil
}

// RefreshToken refreshes the access token
func (c *Client) RefreshToken(ctx context.Context) error {
	c.mu.RLock()
	refreshToken := c.refreshToken
	c.mu.RUnlock()

	if refreshToken == "" {
		return c.Login(ctx)
	}

	payload := map[string]interface{}{
		"refresh_token": refreshToken,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", authURL+"/refresh", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Keyid", c.keyID)
	req.Header.Set("Apikey", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Refresh failed, try full login
		return c.Login(ctx)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return c.Login(ctx)
	}

	if result.AccessToken == "" {
		return c.Login(ctx)
	}

	c.mu.Lock()
	c.accessToken = result.AccessToken
	if result.RefreshToken != "" {
		c.refreshToken = result.RefreshToken
	}
	if result.ExpiresIn > 0 {
		c.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	}
	c.mu.Unlock()

	return nil
}

// ensureToken makes sure we have a valid token
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	needRefresh := c.accessToken == "" || time.Now().After(c.tokenExpiry)
	c.mu.RUnlock()

	if needRefresh {
		return c.RefreshToken(ctx)
	}
	return nil
}

// GetDeviceList retrieves all devices from the Wyze account
func (c *Client) GetDeviceList(ctx context.Context) ([]WyzeDevice, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	accessToken := c.accessToken
	c.mu.RUnlock()

	// Build request
	params := url.Values{}
	params.Set("sv", "9f275790cab94a72bd206c8876429f3c") // API signature
	params.Set("sc", phoneSC)
	params.Set("app_ver", appVer)
	params.Set("ts", fmt.Sprintf("%d", time.Now().Unix()*1000))
	params.Set("access_token", accessToken)
	params.Set("phone_id", phoneID)
	params.Set("app_name", appName)

	reqURL := apiBaseURL + "/app/v2/home_page/get_object_list?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Code   string `json:"code"`
		Msg    string `json:"msg"`
		Data   struct {
			DeviceList []struct {
				MAC           string `json:"mac"`
				Nickname      string `json:"nickname"`
				ProductType   string `json:"product_type"`
				ProductModel  string `json:"product_model"`
				DeviceParams  map[string]interface{} `json:"device_params"`
				FirmwareVer   string `json:"firmware_ver"`
				IsOnline      bool   `json:"is_online"`
				P2PID         string `json:"p2p_id"`
				P2PType       int    `json:"p2p_type"`
				EnrString     string `json:"enr"`
				ParentDtls    string `json:"parent_device_mac"`
				ConnState     int    `json:"conn_state"`
				DeviceChannel int    `json:"device_channel"`
			} `json:"device_list"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse device list: %w", err)
	}

	if result.Code != "1" {
		return nil, fmt.Errorf("API error: %s", result.Msg)
	}

	// Filter for cameras only
	var devices []WyzeDevice
	for _, d := range result.Data.DeviceList {
		if !isCamera(d.ProductType) {
			continue
		}

		device := WyzeDevice{
			MAC:           d.MAC,
			Name:          d.Nickname,
			Model:         d.ProductModel,
			ProductType:   d.ProductType,
			FirmwareVer:   d.FirmwareVer,
			IsOnline:      d.IsOnline,
			P2PID:         d.P2PID,
			P2PType:       d.P2PType,
			EnrString:     d.EnrString,
			ParentMAC:     d.ParentDtls,
			DeviceChannel: d.DeviceChannel,
		}

		// Determine capabilities based on model
		device.Capabilities = getModelCapabilities(d.ProductModel, d.ProductType)

		devices = append(devices, device)
	}

	return devices, nil
}

// isCamera checks if a product type is a camera
func isCamera(productType string) bool {
	cameraTypes := map[string]bool{
		"Camera":         true,
		"WyzeCam":        true,
		"WyzeCamPan":     true,
		"WyzeCamPanV2":   true,
		"WyzeCamV2":      true,
		"WyzeCamV3":      true,
		"WyzeCamV3Pro":   true,
		"WyzeCamOG":      true,
		"WyzeCamOGStack": true,
		"WyzeCamFloodlight": true,
		"WyzeCamDoorbell":   true,
		"WyzeDoorbellPro":   true,
		"WyzeCamOutdoor":    true,
		"WyzeCamOutdoorV2":  true,
	}
	return cameraTypes[productType]
}

// getModelCapabilities returns capabilities based on camera model
func getModelCapabilities(model, productType string) []string {
	caps := []string{"video", "snapshot", "motion"}

	// PTZ cameras
	ptzModels := map[string]bool{
		"WYZECP1": true, // Cam Pan
		"HL_PAN2": true, // Cam Pan v2
		"HL_PAN3": true, // Cam Pan v3
	}
	if ptzModels[model] {
		caps = append(caps, "ptz")
	}

	// Two-way audio (most cameras support this)
	caps = append(caps, "two_way_audio")

	// Doorbell
	if productType == "WyzeCamDoorbell" || productType == "WyzeDoorbellPro" {
		caps = append(caps, "doorbell")
	}

	// Outdoor cameras have battery
	if productType == "WyzeCamOutdoor" || productType == "WyzeCamOutdoorV2" {
		caps = append(caps, "battery")
	}

	// Floodlight cameras
	if productType == "WyzeCamFloodlight" {
		caps = append(caps, "floodlight", "siren")
	}

	// Night vision (all cameras)
	caps = append(caps, "night_vision")

	return caps
}

// GetDeviceInfo gets detailed info for a specific device
func (c *Client) GetDeviceInfo(ctx context.Context, mac string) (*WyzeDevice, error) {
	devices, err := c.GetDeviceList(ctx)
	if err != nil {
		return nil, err
	}

	for _, d := range devices {
		if d.MAC == mac {
			return &d, nil
		}
	}

	return nil, fmt.Errorf("device not found: %s", mac)
}

// RunAction executes an action on a device
func (c *Client) RunAction(ctx context.Context, mac, model, actionKey string, params map[string]interface{}) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	c.mu.RLock()
	accessToken := c.accessToken
	c.mu.RUnlock()

	payload := map[string]interface{}{
		"sv":            "9f275790cab94a72bd206c8876429f3c",
		"sc":            phoneSC,
		"app_ver":       appVer,
		"ts":            time.Now().Unix() * 1000,
		"access_token":  accessToken,
		"phone_id":      phoneID,
		"app_name":      appName,
		"instance_id":   mac,
		"product_model": model,
		"action_key":    actionKey,
		"action_params": params,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiBaseURL+"/app/v2/auto/run_action", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("action failed: %s", resp.Status)
	}

	return nil
}

// WyzeDevice represents a Wyze device
type WyzeDevice struct {
	MAC           string   `json:"mac"`
	Name          string   `json:"name"`
	Model         string   `json:"model"`
	ProductType   string   `json:"product_type"`
	FirmwareVer   string   `json:"firmware_ver"`
	IsOnline      bool     `json:"is_online"`
	P2PID         string   `json:"p2p_id"`
	P2PType       int      `json:"p2p_type"`
	EnrString     string   `json:"enr"`
	ParentMAC     string   `json:"parent_mac"`
	DeviceChannel int      `json:"device_channel"`
	Capabilities  []string `json:"capabilities"`
}
