// Package wordpress provides a Go client library for the WordPress REST API.
package wordpress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-querystring/query"
)

const (
	userAgent          = "go-wordpress"
	headerTotalRecords = "X-WP-Total"
	headerTotalPages   = "X-WP-TotalPages"
	apiPathPrefix      = "/wp/v2"
)

// ErrURLContainsWPV2 is returned from NewClient if URL contains `apiPathPrefix`.
var ErrURLContainsWPV2 = fmt.Errorf("url must not contain %s", apiPathPrefix)

// DefaultHTTPTransport is an http.RoundTripper that has DisableKeepAlives set true.
var DefaultHTTPTransport = &http.Transport{
	DisableKeepAlives: true,
}

// DefaultHTTPClient is an http.Client with the DefaultHTTPTransport and (Cookie) Jar set nil.
var DefaultHTTPClient = &http.Client{
	Jar:       nil,
	Transport: DefaultHTTPTransport,
}

// Error is a generic WordPress error container.
type Error struct {
	Response *http.Response // HTTP response that caused this error
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Data     struct {
		Status int               `json:"status"`
		Params map[string]string `json:"params"`
	} `json:"data"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%v %v: %d %v",
		e.Response.Request.Method, sanitizeURL(e.Response.Request.URL),
		e.Response.StatusCode, e.Message)
}

// Client is a struct containing values and methods used for interacting with the WordPress API.
type Client struct {
	// User agent used when communicating with the WordPress API.
	UserAgent string

	// WordPress timezone location
	Location *time.Location

	// set NonPrettyPermalinks to true if using non-pretty permalinks
	// reference: https://developer.wordpress.org/rest-api/#routes-endpoints
	NonPrettyPermalinks bool

	// if ProcessRawResponseBody is set to true, response from WordPress will be decoded into RawBody filed of response struct
	ProcessRawResponseBody bool

	Categories *CategoriesService
	Comments   *CommentsService
	Media      *MediaService
	Pages      *PagesService
	Posts      *PostsService
	Settings   *SettingsService
	Statuses   *StatusesService
	Tags       *TagsService
	Taxonomies *TaxonomiesService
	Terms      *TermsService
	Types      *TypesService
	Users      *UsersService

	client  *http.Client
	baseURL *url.URL

	common Service // Reuse a single struct instead of allocating one for each service on the heap.
}

type Service struct {
	Client *Client
}

// ListOptions specifies the optional parameters to various List methods that
// support pagination.
type ListOptions struct {
	Context string `url:"context,omitempty"`          // Scope under which the request is made; determines fields present in response.
	Exclude []int  `url:"exclude,omitempty,brackets"` // Ensure result set excludes specific IDs.
	Include []int  `url:"include,omitempty,brackets"` // Limit result set to specific IDs.
	Offset  int    `url:"offset,omitempty"`           // Offset the result set by a specific number of items.
	Order   string `url:"order,omitempty"`            // Order sort attribute ascending or descending.
	OrderBy string `url:"orderby,omitempty"`          // Sort collection by object attribute.
	Page    int    `url:"page,omitempty"`             // Current page of the collection.
	PerPage int    `url:"per_page,omitempty"`         // Maximum number of items to be returned in result set.
	Search  string `url:"search,omitempty"`           // Limit results to those matching a string.
}

// Response is a WordPress REST API response. This wraps the standard http.Response
// returned from WordPress and provides convenient access to things like
// pagination data.
type Response struct {
	*http.Response

	// This value is only used when ProcessRawResponseBody flag of client is turned on
	RawBody interface{}

	// These fields provide the page values for paginating through a set of
	// results. Any or all of these may be set to the zero value for
	// responses that are not part of a paginated set, or for which there
	// are no additional pages.

	TotalRecords int
	TotalPages   int
	CurrentPage  int
	PreviousPage int
	NextPage     int
}

// DeleteResponse is used when deleting an object.
type DeleteResponse struct {
	Deleted  bool            `json:"deleted"`
	Previous json.RawMessage `json:"previous"`
}

// newResponse creates a new Response for the provided http.Response.
// r must not be nil.
func newResponse(r *http.Response) *Response {
	response := &Response{Response: r}
	response.populatePageValues()
	return response
}

// populatePageValues parses the HTTP Link response headers and populates the
// various pagination link values in the Response.
func (r *Response) populatePageValues() {
	totalRecordsHeader := r.Header.Get(headerTotalRecords)
	totalRecords, _ := strconv.Atoi(totalRecordsHeader)
	r.TotalRecords = totalRecords

	totalPagesHeader := r.Header.Get(headerTotalPages)
	totalPages, _ := strconv.Atoi(totalPagesHeader)
	r.TotalPages = totalPages

	currentPage, _ := strconv.Atoi(r.Request.URL.Query().Get("page"))
	if totalRecordsHeader != "" && totalPagesHeader != "" && currentPage == 0 {
		currentPage = 1
	}

	r.CurrentPage = currentPage

	r.PreviousPage = currentPage - 1
	if r.PreviousPage < 0 {
		r.PreviousPage = 0
	}

	r.NextPage = currentPage + 1
	if r.NextPage > r.TotalPages {
		r.NextPage = 0
	}
}

// NewClient returns an initalized Client for the given baseURL and httpClient.
func NewClient(baseURLStr string, httpClient *http.Client) (*Client, error) {
	if strings.Contains(baseURLStr, apiPathPrefix) {
		return nil, ErrURLContainsWPV2
	}

	location := defaultLocation

	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, err
	}

	if !strings.HasSuffix(baseURL.Path, "/") {
		baseURL.Path += "/"
	}

	if httpClient == nil {
		httpClient = DefaultHTTPClient
	}

	c := &Client{
		client:    httpClient,
		UserAgent: userAgent,
		Location:  location,
		baseURL:   baseURL,
	}
	c.common.Client = c
	c.Categories = (*CategoriesService)(&c.common)
	c.Comments = (*CommentsService)(&c.common)
	c.Media = (*MediaService)(&c.common)
	c.Pages = (*PagesService)(&c.common)
	c.Posts = (*PostsService)(&c.common)
	c.Settings = (*SettingsService)(&c.common)
	c.Statuses = (*StatusesService)(&c.common)
	c.Tags = (*TagsService)(&c.common)
	c.Taxonomies = (*TaxonomiesService)(&c.common)
	c.Terms = (*TermsService)(&c.common)
	c.Types = (*TypesService)(&c.common)
	c.Users = (*UsersService)(&c.common)
	return c, nil
}

func (c *Client) getRequestURL(s string) (*url.URL, error) {
	var apiPath string
	if c.NonPrettyPermalinks {
		apiPath = "/?rest_route="
	} else {
		apiPath = "/wp-json"
	}

	if s == "" {
		apiPath += "/"
	} else {
		apiPath = fmt.Sprintf("%s%s%s", apiPath, apiPathPrefix, "/"+s)
	}

	return c.baseURL.Parse(apiPath)
}

// GetCommonService returns a reusable single instance of Service to allocate it to custom services.
func (c *Client) GetCommonService() *Service {
	return &c.common
}

// AddOptions adds the parameters in opt as URL query parameters to s. opt
// must be a struct whose fields may contain "url" tags.
func (c *Client) AddOptions(s string, opt interface{}) (string, error) {
	var connector string
	if c.NonPrettyPermalinks {
		connector = "&"
	} else {
		connector = "?"
	}

	v := reflect.ValueOf(opt)
	if !v.IsValid() || (v.Kind() == reflect.Ptr && v.IsNil()) {
		return s, nil
	}

	if v.Kind() == reflect.String {
		return fmt.Sprintf("%s%s%s", s, connector, opt.(string)), nil
	}

	qs, err := query.Values(opt)
	if err != nil {
		return s, err
	}

	return fmt.Sprintf("%s%s%s", s, connector, qs.Encode()), nil
}

// NewRequest creates an API request. A relative URL can be provided in urlStr,
// in which case it is resolved relative to the baseURL of the Client.
// Relative URLs should always be specified without a preceding slash. If
// specified, the value pointed to by body is JSON encoded and included as the
// request body.
func (c *Client) NewRequest(method, urlStr string, body interface{}) (*http.Request, error) {
	u, err := c.getRequestURL(urlStr)
	if err != nil {
		return nil, err
	}

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		if encErr := enc.Encode(body); encErr != nil {
			return nil, encErr
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	return req, nil
}

// Do sends an API request and returns the API response. The API response is
// JSON decoded and stored in the value pointed to by v, or returned as an
// error if an API error has occurred. If v implements the io.Writer
// interface, the raw response body will be written to v, without attempting to
// first decode it.
//
// The provided ctx must be non-nil. If it is canceled or times out,
// ctx.Err() will be returned.
func (c *Client) Do(ctx context.Context, req *http.Request, v interface{}) (*Response, error) {
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		// If we got an error, and the context has been canceled,
		// the context's error is probably more useful.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// If the error type is *url.Error, sanitize its URL before returning.
		if e, ok := err.(*url.Error); ok {
			if url, urlErr := url.Parse(e.URL); urlErr == nil {
				e.URL = sanitizeURL(url).String()
				return nil, e
			}
		}

		return nil, err
	}

	// nolint: errcheck
	defer func() {
		// Drain up to 512 bytes and close the body to let the Transport reuse the connection
		io.CopyN(ioutil.Discard, resp.Body, 512)
		resp.Body.Close()
	}()

	response := newResponse(resp)

	err = checkResponse(resp)
	if err != nil {
		// even though there was an error, we still return the response
		// in case the caller wants to inspect it further
		return response, err
	}

	if v != nil {
		if w, ok := v.(io.Writer); ok {
			if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
				err = copyErr
			}
		} else {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return response, err
			}

			err = json.Unmarshal(body, v)

			// if ProcessRawResponseBody is turned on, decode body of WordPress response and assign to this function's response
			if c.ProcessRawResponseBody {
				additionalError := json.Unmarshal(body, &response.RawBody)
				if additionalError != nil {
					if err == nil {
						err = additionalError
					} else {
						err = fmt.Errorf("%s\n%s", err, additionalError)
					}
				}
			}
		}
	}

	return response, err
}

// RootInfo is a struct containing basic and publicly available information about the WordPress REST API.
type RootInfo struct {
	Authentication     interface{} `json:"authentication"`
	Description        string      `json:"description"`
	GMTOffset          int         `json:"gmt_offset"`
	HomeURL            string      `json:"home"`
	Name               string      `json:"name"`
	Namespaces         []string    `json:"namespaces"`
	PermalinkStructure string      `json:"permalink_structure"`
	TimezoneString     string      `json:"timezone_string"`
	URL                string      `json:"url"`

	Location *time.Location `json:"-"`
}

// BasicInfo gets basic and publicly available information about the WordPress REST API.
func (c *Client) BasicInfo(ctx context.Context) (*RootInfo, *Response, error) {
	var entity RootInfo

	resp, err := c.Get(ctx, "", nil, &entity)
	if err != nil {
		return &entity, resp, err
	}

	location, locationErr := time.LoadLocation(entity.TimezoneString)
	if locationErr != nil {
		return &entity, resp, locationErr
	}
	entity.Location = location

	return &entity, resp, err
}

// List is a generic function that will return a list of items from the WordPress REST API.
func (c *Client) List(ctx context.Context, url string, params interface{}, result interface{}) (*Response, error) {

	u, err := c.AddOptions(url, params)
	if err != nil {
		return nil, err
	}

	req, err := c.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(ctx, req, &result)
}

// Create creates a new item on the WordPress REST API.
func (c *Client) Create(ctx context.Context, url string, content interface{}, result interface{}) (*Response, error) {
	req, err := c.NewRequest("POST", url, content)
	if err != nil {
		return nil, err
	}

	return c.Do(ctx, req, &result)
}

// Get returns a single item from the WordPress REST API for the given parameters.
func (c *Client) Get(ctx context.Context, url string, params interface{}, result interface{}) (*Response, error) {
	u, err := c.AddOptions(url, params)
	if err != nil {
		return nil, err
	}

	req, err := c.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(ctx, req, &result)
}

// Update will update an item on the WordPress REST API.
func (c *Client) Update(ctx context.Context, url string, content interface{}, result interface{}) (*Response, error) {
	req, err := c.NewRequest("PUT", url, content)
	if err != nil {
		return nil, err
	}

	req.Header.Set("HTTP_X_HTTP_METHOD_OVERRIDE", "PUT")

	return c.Do(ctx, req, &result)
}

// Delete will delete an item from the WordPress REST API.
func (c *Client) Delete(ctx context.Context, url string, params interface{}, result interface{}) (*Response, error) {
	u, err := c.AddOptions(url, params)
	if err != nil {
		return nil, err
	}

	req, err := c.NewRequest("DELETE", u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("HTTP_X_HTTP_METHOD_OVERRIDE", "DELETE")

	if req.URL.Query().Get("force") != "" {
		var deleteResp DeleteResponse

		resp, err := c.Do(ctx, req, &deleteResp)
		if err != nil {
			return resp, err
		}

		if deleteResp.Deleted {
			if err := json.Unmarshal(deleteResp.Previous, &result); err != nil {
				return resp, err
			}
		}

		return resp, nil

	}
	return c.Do(ctx, req, &result)
}

// PostData allows uploading of binary objects to the WordPress REST API.
func (c *Client) PostData(ctx context.Context, urlStr string, content []byte, contentType string, filename string, result interface{}) (*Response, error) {

	// gorequest does not support POST-ing raw data
	// so, we have to manually create a HTTP client
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fileField, fileFieldErr := w.CreateFormFile("file", filename)
	if fileFieldErr != nil {
		return nil, fileFieldErr
	}
	if _, writeErr := fileField.Write(content); writeErr != nil {
		return nil, writeErr
	}
	if closeErr := w.Close(); closeErr != nil {
		return nil, closeErr
	}

	u, err := c.getRequestURL(urlStr)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", u.String(), &buf)
	if err != nil {
		return nil, err
	}

	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Content-Disposition", fmt.Sprintf("filename=%v", filename))

	// Send request
	return c.Do(ctx, req, &result)
}

// sanitizeURL redacts the password parameter from the URL which may be
// exposed to the user.
func sanitizeURL(uri *url.URL) *url.URL {
	if uri == nil {
		return nil
	}
	params := uri.Query()
	if len(params.Get("password")) > 0 {
		params.Set("password", "REDACTED")
		uri.RawQuery = params.Encode()
	}
	return uri
}

// checkResponse checks the API response for errors, and returns them if
// present. A response is considered an error if it has a status code outside
// the 200 range or equal to 202 Accepted.
// API error responses are expected to have either no response
// body, or a JSON response body that maps to ErrorResponse. Any other
// response body will be silently ignored.
func checkResponse(r *http.Response) error {
	if c := r.StatusCode; 200 <= c && c <= 299 {
		return nil
	}
	errorResponse := &Error{Response: r}
	data, err := ioutil.ReadAll(r.Body)
	if err == nil && data != nil {
		if jsonErr := json.Unmarshal(data, errorResponse); jsonErr != nil {
			return jsonErr
		}
	}
	return errorResponse
}
