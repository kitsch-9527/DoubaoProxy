package doubao

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/google/uuid"

	"DoubaoProxy/internal/model"
	"DoubaoProxy/internal/session"
)

// UploadFile 完整复刻 Python 版本的四步上传流程。
func (s *Service) UploadFile(ctx context.Context, fileType int, fileName string, fileBytes []byte) (*model.UploadResponse, error) {
	if len(fileBytes) == 0 {
		return nil, model.NewHTTPError(http.StatusBadRequest, "file_bytes body is empty")
	}
	if filepath.Ext(fileName) == "" {
		return nil, model.NewHTTPError(http.StatusBadRequest, "file_name must include an extension")
	}

	session, err := s.pool.GetSession("", false)
	if err != nil {
		return nil, err
	}

	info, err := s.prepareUpload(ctx, session, fileType)
	if err != nil {
		return nil, err
	}

	creds := aws.Credentials{
		AccessKeyID:     info.Auth.AccessKey,
		SecretAccessKey: info.Auth.SecretKey,
		SessionToken:    info.Auth.SessionToken,
		Source:          "doubao",
	}

	apply, err := s.applyUpload(ctx, creds, info.ServiceID, fileName, len(fileBytes))
	if err != nil {
		return nil, err
	}

	if err := s.uploadToStore(ctx, apply.StoreURI, apply.StoreAuth, fileBytes); err != nil {
		return nil, err
	}

	result, err := s.commitUpload(ctx, creds, info.ServiceID, apply.SessionKey)
	if err != nil {
		return nil, err
	}

	return buildUploadResponse(fileType, fileName, fileBytes, result), nil
}

type uploadAuth struct {
	SessionToken string
	AccessKey    string
	SecretKey    string
}

type prepareInfo struct {
	ServiceID string
	Auth      uploadAuth
}

type applyInfo struct {
	StoreURI   string
	StoreAuth  string
	SessionKey string
}

type commitResult struct {
	ImageURI    string
	ImageMD5    string
	ImageSize   int
	ImageWidth  int
	ImageHeight int
}

type prepareResponse struct {
	Data struct {
		ServiceID       string `json:"service_id"`
		UploadAuthToken struct {
			SessionToken string `json:"session_token"`
			AccessKey    string `json:"access_key"`
			SecretKey    string `json:"secret_key"`
		} `json:"upload_auth_token"`
	} `json:"data"`
}

type applyResponse struct {
	Result struct {
		UploadAddress struct {
			StoreInfos []struct {
				StoreURI string `json:"StoreUri"`
				Auth     string `json:"Auth"`
			} `json:"StoreInfos"`
			SessionKey string `json:"SessionKey"`
		} `json:"UploadAddress"`
	} `json:"Result"`
}

type commitResponse struct {
	Result struct {
		PluginResult []struct {
			ImageURI    string `json:"ImageUri"`
			ImageMd5    string `json:"ImageMd5"`
			ImageSize   int    `json:"ImageSize"`
			ImageWidth  int    `json:"ImageWidth"`
			ImageHeight int    `json:"ImageHeight"`
		} `json:"PluginResult"`
	} `json:"Result"`
}

type uploadAck struct {
	Message string `json:"message"`
}

func (s *Service) prepareUpload(ctx context.Context, sess *session.Session, fileType int) (*prepareInfo, error) {
	values := url.Values{}
	values.Set("aid", "497858")
	values.Set("device_id", sess.DeviceID)
	values.Set("device_platform", "web")
	values.Set("language", "zh")
	values.Set("pc_version", "2.20.0")
	values.Set("pkg_type", "release_version")
	values.Set("real_aid", "497858")
	values.Set("region", "CN")
	values.Set("samantha_web", "1")
	values.Set("sys_region", "CN")
	values.Set("tea_uuid", sess.TeaUUID)
	values.Set("use-olympus-account", "1")
	values.Set("version_code", "20800")
	values.Set("web_id", sess.WebID)

	endpoint := "https://www.doubao.com/alice/resource/prepare_upload?" + values.Encode()
	payload := map[string]any{
		"resource_type": fileType,
		"scene_id":      "5",
		"tenant_id":     "5",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal prepare payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create prepare request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", sess.Cookie)
	req.Header.Set("Origin", "https://www.doubao.com")
	req.Header.Set("Referer", "https://www.doubao.com/chat/")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call prepare_upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, model.NewHTTPError(resp.StatusCode, "prepare_upload failed: %s", strings.TrimSpace(string(body)))
	}

	var pr prepareResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode prepare_upload response: %w", err)
	}
	info := &prepareInfo{
		ServiceID: pr.Data.ServiceID,
		Auth: uploadAuth{
			SessionToken: pr.Data.UploadAuthToken.SessionToken,
			AccessKey:    pr.Data.UploadAuthToken.AccessKey,
			SecretKey:    pr.Data.UploadAuthToken.SecretKey,
		},
	}
	if info.ServiceID == "" || info.Auth.AccessKey == "" || info.Auth.SecretKey == "" {
		return nil, model.NewHTTPError(http.StatusBadGateway, "prepare_upload returned empty auth info")
	}
	return info, nil
}

func (s *Service) applyUpload(ctx context.Context, creds aws.Credentials, serviceID, fileName string, fileSize int) (*applyInfo, error) {
	ext := filepath.Ext(fileName)
	if ext == "" {
		return nil, model.NewHTTPError(http.StatusBadRequest, "file_name must include an extension")
	}

	endpoint := fmt.Sprintf("https://imagex.bytedanceapi.com/?Action=ApplyImageUpload&Version=2018-08-01&ServiceId=%s&NeedFallback=true&FileSize=%d&FileExtension=%s", serviceID, fileSize, ext)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create apply upload request: %w", err)
	}
	req.Header.Set("Origin", "https://www.doubao.com")
	req.Header.Set("Referer", "https://www.doubao.com")
	req.Header.Set("User-Agent", defaultUserAgent)

	if err := signAWSRequest(ctx, req, creds, ""); err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call apply_upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, model.NewHTTPError(resp.StatusCode, "apply_upload failed: %s", strings.TrimSpace(string(body)))
	}

	var ar applyResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("decode apply_upload response: %w", err)
	}

	if len(ar.Result.UploadAddress.StoreInfos) == 0 {
		return nil, model.NewHTTPError(http.StatusBadGateway, "apply_upload returned empty StoreInfos")
	}

	store := ar.Result.UploadAddress.StoreInfos[0]
	return &applyInfo{
		StoreURI:   store.StoreURI,
		StoreAuth:  store.Auth,
		SessionKey: ar.Result.UploadAddress.SessionKey,
	}, nil
}

func (s *Service) uploadToStore(ctx context.Context, storeURI, auth string, fileBytes []byte) error {
	if storeURI == "" {
		return model.NewHTTPError(http.StatusBadGateway, "store uri missing from apply_upload response")
	}

	endpoint := fmt.Sprintf("https://tos-d-x-hl.snssdk.com/upload/v1/%s", storeURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(fileBytes))
	if err != nil {
		return fmt.Errorf("create store upload request: %w", err)
	}

	crc := crc32.ChecksumIEEE(fileBytes)
	crcHex := fmt.Sprintf("%08x", crc)

	req.Header.Set("Authorization", auth)
	req.Header.Set("Origin", "https://www.doubao.com")
	req.Header.Set("Referer", "https://www.doubao.com")
	req.Header.Set("Host", "tos-d-x-hl.snssdk.com")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Disposition", "attachment; filename=\"undefined\"")
	req.Header.Set("Content-Crc32", crcHex)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call store upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return model.NewHTTPError(resp.StatusCode, "store upload failed: %s", strings.TrimSpace(string(body)))
	}

	var ack uploadAck
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		return fmt.Errorf("decode store upload response: %w", err)
	}
	if strings.ToLower(ack.Message) != "success" {
		return model.NewHTTPError(http.StatusBadGateway, "store upload returned: %s", ack.Message)
	}
	return nil
}

func (s *Service) commitUpload(ctx context.Context, creds aws.Credentials, serviceID, sessionKey string) (*commitResult, error) {
	payload := map[string]string{"SessionKey": sessionKey}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal commit payload: %w", err)
	}

	endpoint := fmt.Sprintf("https://imagex.bytedanceapi.com/?Action=CommitImageUpload&Version=2018-08-01&ServiceId=%s", serviceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create commit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://www.doubao.com")
	req.Header.Set("Referer", "https://www.doubao.com/")
	req.Header.Set("User-Agent", defaultUserAgent)

	hash := sha256.Sum256(data)
	if err := signAWSRequest(ctx, req, creds, hex.EncodeToString(hash[:])); err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call commit_upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, model.NewHTTPError(resp.StatusCode, "commit_upload failed: %s", strings.TrimSpace(string(body)))
	}

	var cr commitResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("decode commit_upload response: %w", err)
	}
	if len(cr.Result.PluginResult) == 0 {
		return nil, model.NewHTTPError(http.StatusBadGateway, "commit_upload PluginResult empty")
	}

	first := cr.Result.PluginResult[0]
	return &commitResult{
		ImageURI:    first.ImageURI,
		ImageMD5:    first.ImageMd5,
		ImageSize:   first.ImageSize,
		ImageWidth:  first.ImageWidth,
		ImageHeight: first.ImageHeight,
	}, nil
}

func buildUploadResponse(fileType int, fileName string, fileBytes []byte, result *commitResult) *model.UploadResponse {
	identifier := uuid.NewString()
	base := &model.UploadResponse{
		Key:        result.ImageURI,
		Name:       fileName,
		Identifier: identifier,
		MD5:        fallbackMD5(result.ImageMD5, fileBytes),
		Size:       fallbackInt(result.ImageSize, len(fileBytes)),
	}

	if fileType == 2 {
		base.Type = "vlm_image"
		base.FileReviewState = 3
		base.FileParseState = 3
		base.Option = map[string]any{
			"height": fallbackInt(result.ImageHeight, 0),
			"width":  fallbackInt(result.ImageWidth, 0),
		}
		return base
	}

	base.Type = "file"
	base.FileReviewState = 1
	base.FileParseState = 3
	return base
}

func fallbackMD5(existing string, data []byte) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func fallbackInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func signAWSRequest(ctx context.Context, req *http.Request, creds aws.Credentials, payloadHash string) error {
	signer := v4.NewSigner()
	if payloadHash == "" {
		payloadHash = emptyPayloadHash
	}
	if err := signer.SignHTTP(ctx, creds, req, payloadHash, "imagex", "cn-north-1", time.Now()); err != nil {
		return fmt.Errorf("sign request: %w", err)
	}
	return nil
}
