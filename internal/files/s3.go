package files

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type s3Storage struct {
	endpoint                       *url.URL
	region, bucket, access, secret string
	client                         *http.Client
}

func newS3Storage(o Options) *s3Storage {
	u, _ := url.Parse(strings.TrimRight(o.S3Endpoint, "/"))
	return &s3Storage{endpoint: u, region: o.S3Region, bucket: o.S3Bucket, access: o.S3AccessKey, secret: o.S3SecretKey, client: &http.Client{Timeout: 30 * time.Second}}
}
func (s *s3Storage) objectURL(key string) *url.URL {
	u := *s.endpoint
	u.Path = path.Join(u.Path, s.bucket, key)
	return &u
}
func (s *s3Storage) request(ctx context.Context, method, key string, body io.Reader, payloadHash, contentType string) (*http.Response, error) {
	u := s.objectURL(key)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	s.sign(req, payloadHash, time.Now().UTC())
	return s.client.Do(req)
}
func (s *s3Storage) Put(ctx context.Context, key, staged, contentType string) error {
	file, err := os.Open(staged)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if _, err = file.Seek(0, 0); err != nil {
		return err
	}
	response, err := s.request(ctx, http.MethodPut, key, file, hex.EncodeToString(hash.Sum(nil)), contentType)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("s3 put: %s", response.Status)
	}
	return os.Remove(staged)
}
func (s *s3Storage) Serve(w http.ResponseWriter, r *http.Request, key string, m Metadata) error {
	response, err := s.request(r.Context(), http.MethodGet, key, nil, emptySHA(), "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("s3 get: %s", response.Status)
	}
	w.Header().Set("Content-Type", m.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="download"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	_, err = io.Copy(w, io.LimitReader(response.Body, MaxUpload+1))
	return err
}
func (s *s3Storage) Delete(ctx context.Context, key string) error {
	response, err := s.request(ctx, http.MethodDelete, key, nil, emptySHA(), "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 && response.StatusCode != 404 {
		return fmt.Errorf("s3 delete: %s", response.Status)
	}
	return nil
}
func (s *s3Storage) Cleanup(ctx context.Context, known map[string]bool) (int, error) {
	u := s.objectURL("")
	u.RawQuery = "list-type=2"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	s.sign(req, emptySHA(), time.Now().UTC())
	response, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return 0, fmt.Errorf("s3 list: %s", response.Status)
	}
	var listing struct {
		Contents []struct {
			Key string `xml:"Key"`
		} `xml:"Contents"`
	}
	if decodeErr := xml.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&listing); decodeErr != nil {
		return 0, decodeErr
	}
	removed := 0
	for _, item := range listing.Contents {
		if !known[item.Key] && s.Delete(ctx, item.Key) == nil {
			removed++
		}
	}
	return removed, nil
}
func (s *s3Storage) sign(req *http.Request, payloadHash string, now time.Time) {
	date := now.Format("20060102")
	stamp := now.Format("20060102T150405Z")
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	names := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	sort.Strings(names)
	canonicalHeaders := "host:" + req.URL.Host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + stamp + "\n"
	canonical := req.Method + "\n" + req.URL.EscapedPath() + "\n" + req.URL.Query().Encode() + "\n" + canonicalHeaders + "\n" + strings.Join(names, ";") + "\n" + payloadHash
	scope := date + "/" + s.region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := hmacSHA(hmacSHA(hmacSHA(hmacSHA([]byte("AWS4"+s.secret), date), s.region), "s3"), "aws4_request")
	signature := hex.EncodeToString(hmacSHA(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.access+"/"+scope+", SignedHeaders="+strings.Join(names, ";")+", Signature="+signature)
}
func hmacSHA(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(value))
	return h.Sum(nil)
}
func emptySHA() string { sum := sha256.Sum256(nil); return hex.EncodeToString(sum[:]) }
