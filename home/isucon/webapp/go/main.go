package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

const (
	sessionName                 = "isucondition_go"
	conditionLimit              = 20
	frontendContentsPath        = "../public"
	jiaJWTSigningKeyPath        = "../ec256-public.pem"
	defaultIconFilePath         = "../NoImage.jpg"
	isuIconCacheDir             = "/home/isucon/webapp/public/.isucondition-icon-cache"
	isuIconInternalPath         = "/_isucondition_icon_cache/"
	defaultJIAServiceURL        = "http://localhost:5000"
	mysqlErrNumDuplicateEntry   = 1062
	sessionCacheTTL             = 30 * time.Second
	sessionCacheMaxEntries      = 10000
	dropConditionLogIdle        = 10 * time.Second
	conditionLevelInfo          = "info"
	conditionLevelWarning       = "warning"
	conditionLevelCritical      = "critical"
	scoreConditionLevelInfo     = 3
	scoreConditionLevelWarning  = 2
	scoreConditionLevelCritical = 1
)

var (
	db                  *sqlx.DB
	sessionStore        sessions.Store
	mySQLConnectionData *MySQLConnectionEnv

	jiaJWTSigningKey *ecdsa.PublicKey

	postIsuConditionTargetBaseURL string // JIAへのactivate時に登録する，ISUがconditionを送る先のURL
	trendCache                    struct {
		sync.RWMutex
		response []TrendResponse
		valid    bool
	}
	sessionCache struct {
		sync.Mutex
		entries     map[string]sessionCacheEntry
		nextCleanup time.Time
	}
	dropConditionPending uint64
	dropConditionTotal   uint64
	dropConditionLogger  echo.Logger
	prettyJSONBufPool    = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
	// 存在する jia_isu_uuid だけを持つ正のキャッシュ。未ヒット（404）は載せない。
	isuUUIDCache sync.Map
	// ディスクに書いたアイコンの正のキャッシュ。未ヒットは載せない。
	iconFileCache sync.Map
	graphCache    struct {
		sync.Mutex
		entries map[graphCacheKey][]byte
	}
)

func invalidateTrendCache() {
	trendCache.Lock()
	trendCache.valid = false
	trendCache.response = nil
	trendCache.Unlock()
}

func rememberIsuUUID(jiaIsuUUID string) {
	isuUUIDCache.Store(jiaIsuUUID, struct{}{})
}

func invalidateIsuUUIDCache() {
	isuUUIDCache.Range(func(key, _ interface{}) bool {
		isuUUIDCache.Delete(key)
		return true
	})
}

func buildIsuConditionInsertValues(n int) string {
	placeholders := make([]string, n)
	for i := 0; i < n; i++ {
		placeholders[i] = "(?, ?, ?, ?, ?)"
	}
	return strings.Join(placeholders, ",")
}

type sessionCacheEntry struct {
	jiaUserID string
	expiresAt time.Time
}

func sessionCookieValue(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func getCachedSessionUserID(cookieValue string) (string, bool) {
	now := time.Now()
	sessionCache.Lock()
	defer sessionCache.Unlock()

	entry, ok := sessionCache.entries[cookieValue]
	if !ok || !entry.expiresAt.After(now) {
		if ok {
			delete(sessionCache.entries, cookieValue)
		}
		return "", false
	}
	return entry.jiaUserID, true
}

func cacheSessionUserID(cookieValue, jiaUserID string) {
	now := time.Now()
	sessionCache.Lock()
	defer sessionCache.Unlock()

	if sessionCache.entries == nil || len(sessionCache.entries) >= sessionCacheMaxEntries {
		sessionCache.entries = make(map[string]sessionCacheEntry)
	}
	if now.After(sessionCache.nextCleanup) {
		for key, entry := range sessionCache.entries {
			if !entry.expiresAt.After(now) {
				delete(sessionCache.entries, key)
			}
		}
		sessionCache.nextCleanup = now.Add(sessionCacheTTL)
	}
	sessionCache.entries[cookieValue] = sessionCacheEntry{
		jiaUserID: jiaUserID,
		expiresAt: now.Add(sessionCacheTTL),
	}
}

func deleteCachedSessionUserID(cookieValue string) {
	sessionCache.Lock()
	delete(sessionCache.entries, cookieValue)
	sessionCache.Unlock()
}

func invalidateSessionCache() {
	sessionCache.Lock()
	sessionCache.entries = nil
	sessionCache.nextCleanup = time.Time{}
	sessionCache.Unlock()
}

// writeJSON は Echo 4.3.0 が e.Debug=true のとき c.JSON で出す pretty JSON と
// 同じバイト列を書く。json.Encoder.SetIndent の compact marshal + Indent 再パースを避け、
// json/v2 で 2 スペース indent を1パスで出す。末尾改行は Encoder.Encode に合わせる。
func writeJSON(c echo.Context, code int, v interface{}) error {
	body, err := marshalPrettyJSON(v)
	if err != nil {
		return err
	}
	return writeJSONBytes(c, code, body)
}

func marshalPrettyJSON(v interface{}) ([]byte, error) {
	buf := prettyJSONBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer prettyJSONBufPool.Put(buf)

	if err := jsonv2.MarshalWrite(buf, v, json.DefaultOptionsV1(), jsontext.WithIndent("  ")); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

func writeJSONBytes(c echo.Context, code int, body []byte) error {
	res := c.Response()
	if res.Header().Get(echo.HeaderContentType) == "" {
		res.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSONCharsetUTF8)
	}
	res.WriteHeader(code)
	_, err := res.Write(body)
	return err
}

func isuIconCachePath(jiaUserID, jiaIsuUUID string) string {
	digest := sha256.Sum256([]byte(jiaUserID + "\x00" + jiaIsuUUID))
	return filepath.Join(isuIconCacheDir, fmt.Sprintf("%x", digest))
}

func cachedIconKey(jiaUserID, jiaIsuUUID string) string {
	return jiaUserID + "\x00" + jiaIsuUUID
}

func rememberCachedIcon(jiaUserID, jiaIsuUUID string) {
	iconFileCache.Store(cachedIconKey(jiaUserID, jiaIsuUUID), struct{}{})
}

func hasCachedIcon(jiaUserID, jiaIsuUUID string) bool {
	_, ok := iconFileCache.Load(cachedIconKey(jiaUserID, jiaIsuUUID))
	return ok
}

func invalidateCachedIcon() {
	iconFileCache.Range(func(key, _ interface{}) bool {
		iconFileCache.Delete(key)
		return true
	})
}

func cacheIsuIcon(jiaUserID, jiaIsuUUID string, image []byte) (string, error) {
	if err := os.MkdirAll(isuIconCacheDir, 0755); err != nil {
		return "", err
	}

	path := isuIconCachePath(jiaUserID, jiaIsuUUID)
	file, err := os.CreateTemp(isuIconCacheDir, ".tmp-*")
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if _, err := file.Write(image); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Chmod(0644); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return "", err
	}
	rememberCachedIcon(jiaUserID, jiaIsuUUID)
	return path, nil
}

func serveCachedIsuIcon(c echo.Context, jiaUserID, jiaIsuUUID string) bool {
	path := isuIconCachePath(jiaUserID, jiaIsuUUID)
	if !hasCachedIcon(jiaUserID, jiaIsuUUID) {
		if _, err := os.Stat(path); err != nil {
			return false
		}
		rememberCachedIcon(jiaUserID, jiaIsuUUID)
	}
	c.Response().Header().Set("X-Accel-Redirect", isuIconInternalPath+filepath.Base(path))
	c.Response().Header().Set("Cache-Control", "private, max-age=60, must-revalidate")
	return true
}

type Config struct {
	Name string `db:"name"`
	URL  string `db:"url"`
}

type Isu struct {
	ID         int       `db:"id" json:"id"`
	JIAIsuUUID string    `db:"jia_isu_uuid" json:"jia_isu_uuid"`
	Name       string    `db:"name" json:"name"`
	Image      []byte    `db:"image" json:"-"`
	Character  string    `db:"character" json:"character"`
	JIAUserID  string    `db:"jia_user_id" json:"-"`
	CreatedAt  time.Time `db:"created_at" json:"-"`
	UpdatedAt  time.Time `db:"updated_at" json:"-"`
}

type IsuListRow struct {
	ID            int            `db:"id"`
	JIAIsuUUID    string         `db:"jia_isu_uuid"`
	Name          string         `db:"name"`
	Character     string         `db:"character"`
	LastTimestamp *time.Time     `db:"last_timestamp"`
	LastIsSitting sql.NullBool   `db:"last_is_sitting"`
	LastCondition sql.NullString `db:"last_condition"`
	LastMessage   sql.NullString `db:"last_message"`
}

type IsuFromJIA struct {
	Character string `json:"character"`
}

type GetIsuListResponse struct {
	ID                 int                      `json:"id"`
	JIAIsuUUID         string                   `json:"jia_isu_uuid"`
	Name               string                   `json:"name"`
	Character          string                   `json:"character"`
	LatestIsuCondition *GetIsuConditionResponse `json:"latest_isu_condition"`
}

type IsuCondition struct {
	ID         int       `db:"id"`
	JIAIsuUUID string    `db:"jia_isu_uuid"`
	Timestamp  time.Time `db:"timestamp"`
	IsSitting  bool      `db:"is_sitting"`
	Condition  string    `db:"condition"`
	Message    string    `db:"message"`
	CreatedAt  time.Time `db:"created_at"`
}

type MySQLConnectionEnv struct {
	Host     string
	Port     string
	User     string
	DBName   string
	Password string
}

type InitializeRequest struct {
	JIAServiceURL string `json:"jia_service_url"`
}

type InitializeResponse struct {
	Language string `json:"language"`
}

type GetMeResponse struct {
	JIAUserID string `json:"jia_user_id"`
}

type GraphResponse struct {
	StartAt             int64           `json:"start_at"`
	EndAt               int64           `json:"end_at"`
	Data                *GraphDataPoint `json:"data"`
	ConditionTimestamps []int64         `json:"condition_timestamps"`
}

type GraphDataPoint struct {
	Score      int                  `json:"score"`
	Percentage ConditionsPercentage `json:"percentage"`
}

type ConditionsPercentage struct {
	Sitting      int `json:"sitting"`
	IsBroken     int `json:"is_broken"`
	IsDirty      int `json:"is_dirty"`
	IsOverweight int `json:"is_overweight"`
}

type GraphDataPointWithInfo struct {
	JIAIsuUUID          string
	StartAt             time.Time
	Data                GraphDataPoint
	ConditionTimestamps []int64
}

type GetIsuConditionResponse struct {
	JIAIsuUUID     string `json:"jia_isu_uuid"`
	IsuName        string `json:"isu_name"`
	Timestamp      int64  `json:"timestamp"`
	IsSitting      bool   `json:"is_sitting"`
	Condition      string `json:"condition"`
	ConditionLevel string `json:"condition_level"`
	Message        string `json:"message"`
}

type TrendResponse struct {
	Character string            `json:"character"`
	Info      []*TrendCondition `json:"info"`
	Warning   []*TrendCondition `json:"warning"`
	Critical  []*TrendCondition `json:"critical"`
}

type TrendCondition struct {
	ID        int   `json:"isu_id"`
	Timestamp int64 `json:"timestamp"`
}

type TrendConditionRow struct {
	ID        int       `db:"id"`
	Character string    `db:"character"`
	Timestamp time.Time `db:"timestamp"`
	Condition string    `db:"condition"`
}

type PostIsuConditionRequest struct {
	IsSitting bool   `json:"is_sitting"`
	Condition string `json:"condition"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

type JIAServiceRequest struct {
	TargetBaseURL string `json:"target_base_url"`
	IsuUUID       string `json:"isu_uuid"`
}

func getEnv(key string, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

func NewMySQLConnectionEnv() *MySQLConnectionEnv {
	return &MySQLConnectionEnv{
		Host:     getEnv("MYSQL_HOST", "127.0.0.1"),
		Port:     getEnv("MYSQL_PORT", "3306"),
		User:     getEnv("MYSQL_USER", "isucon"),
		DBName:   getEnv("MYSQL_DBNAME", "isucondition"),
		Password: getEnv("MYSQL_PASS", "isucon"),
	}
}

func (mc *MySQLConnectionEnv) ConnectDB() (*sqlx.DB, error) {
	dsn := fmt.Sprintf("%v:%v@tcp(%v:%v)/%v?parseTime=true&loc=Asia%%2FTokyo&interpolateParams=true", mc.User, mc.Password, mc.Host, mc.Port, mc.DBName)
	return sqlx.Open("mysql", dsn)
}

func init() {
	sessionStore = sessions.NewCookieStore([]byte(getEnv("SESSION_KEY", "isucondition")))

	key, err := ioutil.ReadFile(jiaJWTSigningKeyPath)
	if err != nil {
		log.Fatalf("failed to read file: %v", err)
	}
	jiaJWTSigningKey, err = jwt.ParseECPublicKeyFromPEM(key)
	if err != nil {
		log.Fatalf("failed to parse ECDSA public key: %v", err)
	}
}

func main() {
	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.DEBUG)
	dropConditionLogger = e.Logger
	startDropConditionLogFlusher()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	if getEnv("ENABLE_PPROF", "0") == "1" {
		e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
	}

	e.POST("/initialize", postInitialize)

	e.POST("/api/auth", postAuthentication)
	e.POST("/api/signout", postSignout)
	e.GET("/api/user/me", getMe)
	e.GET("/api/isu", getIsuList)
	e.POST("/api/isu", postIsu)
	e.GET("/api/isu/:jia_isu_uuid", getIsuID)
	e.GET("/api/isu/:jia_isu_uuid/icon", getIsuIcon)
	e.GET("/api/isu/:jia_isu_uuid/graph", getIsuGraph)
	e.GET("/api/condition/:jia_isu_uuid", getIsuConditions)
	e.GET("/api/trend", getTrend)

	e.POST("/api/condition/:jia_isu_uuid", postIsuCondition)

	e.GET("/", getIndex)
	e.GET("/isu/:jia_isu_uuid", getIndex)
	e.GET("/isu/:jia_isu_uuid/condition", getIndex)
	e.GET("/isu/:jia_isu_uuid/graph", getIndex)
	e.GET("/register", getIndex)
	e.Static("/assets", frontendContentsPath+"/assets")

	mySQLConnectionData = NewMySQLConnectionEnv()

	var err error
	db, err = mySQLConnectionData.ConnectDB()
	if err != nil {
		e.Logger.Fatalf("failed to connect db: %v", err)
		return
	}
	db.SetMaxOpenConns(10)
	defer db.Close()

	postIsuConditionTargetBaseURL = os.Getenv("POST_ISUCONDITION_TARGET_BASE_URL")
	if postIsuConditionTargetBaseURL == "" {
		e.Logger.Fatalf("missing: POST_ISUCONDITION_TARGET_BASE_URL")
		return
	}

	serverPort := fmt.Sprintf(":%v", getEnv("SERVER_APP_PORT", "3000"))
	e.Logger.Fatal(e.Start(serverPort))
}

func getSession(r *http.Request) (*sessions.Session, error) {
	session, err := sessionStore.Get(r, sessionName)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func getUserIDFromSession(c echo.Context) (string, int, error) {
	cookieValue, hasCookie := sessionCookieValue(c.Request())
	if hasCookie {
		if jiaUserID, ok := getCachedSessionUserID(cookieValue); ok {
			return jiaUserID, 0, nil
		}
	}

	session, err := getSession(c.Request())
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("failed to get session: %v", err)
	}
	_jiaUserID, ok := session.Values["jia_user_id"]
	if !ok {
		return "", http.StatusUnauthorized, fmt.Errorf("no session")
	}

	jiaUserID := _jiaUserID.(string)
	var count int

	err = db.Get(&count, "SELECT COUNT(*) FROM `user` WHERE `jia_user_id` = ?",
		jiaUserID)
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("db error: %v", err)
	}

	if count == 0 {
		return "", http.StatusUnauthorized, fmt.Errorf("not found: user")
	}
	if hasCookie {
		cacheSessionUserID(cookieValue, jiaUserID)
	}

	return jiaUserID, 0, nil
}

func getJIAServiceURL(db *sqlx.DB) string {
	var config Config
	err := db.Get(&config, "SELECT * FROM `isu_association_config` WHERE `name` = ?", "jia_service_url")
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Print(err)
		}
		return defaultJIAServiceURL
	}
	return config.URL
}

// POST /initialize
// サービスを初期化
func postInitialize(c echo.Context) error {
	var request InitializeRequest
	err := c.Bind(&request)
	if err != nil {
		return c.String(http.StatusBadRequest, "bad request body")
	}

	cmd := exec.Command("../sql/init.sh")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	err = cmd.Run()
	if err != nil {
		c.Logger().Errorf("exec init.sh error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, err = db.Exec(
		"INSERT INTO `isu_association_config` (`name`, `url`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `url` = VALUES(`url`)",
		"jia_service_url",
		request.JIAServiceURL,
	)
	if err != nil {
		c.Logger().Errorf("db error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	invalidateTrendCache()
	invalidateSessionCache()
	invalidateIsuUUIDCache()
	invalidateIsuMetaCache()
	invalidateGraphCache()
	invalidateCachedIcon()
	if err := os.RemoveAll(isuIconCacheDir); err != nil {
		c.Logger().Errorf("failed to clear icon cache: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return writeJSON(c, http.StatusOK, InitializeResponse{
		Language: "go",
	})
}

// POST /api/auth
// サインアップ・サインイン
func postAuthentication(c echo.Context) error {
	reqJwt := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")

	token, err := jwt.Parse(reqJwt, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, jwt.NewValidationError(fmt.Sprintf("unexpected signing method: %v", token.Header["alg"]), jwt.ValidationErrorSignatureInvalid)
		}
		return jiaJWTSigningKey, nil
	})
	if err != nil {
		switch err.(type) {
		case *jwt.ValidationError:
			return c.String(http.StatusForbidden, "forbidden")
		default:
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.Logger().Errorf("invalid JWT payload")
		return c.NoContent(http.StatusInternalServerError)
	}
	jiaUserIDVar, ok := claims["jia_user_id"]
	if !ok {
		return c.String(http.StatusBadRequest, "invalid JWT payload")
	}
	jiaUserID, ok := jiaUserIDVar.(string)
	if !ok {
		return c.String(http.StatusBadRequest, "invalid JWT payload")
	}

	_, err = db.Exec("INSERT IGNORE INTO user (`jia_user_id`) VALUES (?)", jiaUserID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	session, err := getSession(c.Request())
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	session.Values["jia_user_id"] = jiaUserID
	err = session.Save(c.Request(), c.Response())
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

// POST /api/signout
// サインアウト
func postSignout(c echo.Context) error {
	_, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	session, err := getSession(c.Request())
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	session.Options = &sessions.Options{MaxAge: -1, Path: "/"}
	err = session.Save(c.Request(), c.Response())
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if cookieValue, ok := sessionCookieValue(c.Request()); ok {
		deleteCachedSessionUserID(cookieValue)
	}

	return c.NoContent(http.StatusOK)
}

// GET /api/user/me
// サインインしている自分自身の情報を取得
func getMe(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	res := GetMeResponse{JIAUserID: jiaUserID}
	return writeJSON(c, http.StatusOK, res)
}

// GET /api/isu
// ISUの一覧を取得
func getIsuList(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	metas, err := listIsuMetaByUser(jiaUserID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	uuids := make([]string, 0, len(metas))
	for _, meta := range metas {
		uuids = append(uuids, meta.JIAIsuUUID)
	}
	condRows, err := latestConditionsByUUIDs(uuids)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	condByUUID := make(map[string]latestConditionRow, len(condRows))
	for _, row := range condRows {
		condByUUID[row.JIAIsuUUID] = row
	}

	responseList := make([]GetIsuListResponse, 0, len(metas))
	for _, meta := range metas {
		var formattedCondition *GetIsuConditionResponse
		if row, ok := condByUUID[meta.JIAIsuUUID]; ok && row.Timestamp != nil {
			conditionLevel, err := calculateConditionLevel(row.Condition.String)
			if err != nil {
				c.Logger().Error(err)
				return c.NoContent(http.StatusInternalServerError)
			}

			formattedCondition = &GetIsuConditionResponse{
				JIAIsuUUID:     meta.JIAIsuUUID,
				IsuName:        meta.Name,
				Timestamp:      row.Timestamp.Unix(),
				IsSitting:      row.IsSitting.Bool,
				Condition:      row.Condition.String,
				ConditionLevel: conditionLevel,
				Message:        row.Message.String,
			}
		}

		responseList = append(responseList, GetIsuListResponse{
			ID:                 meta.ID,
			JIAIsuUUID:         meta.JIAIsuUUID,
			Name:               meta.Name,
			Character:          meta.Character,
			LatestIsuCondition: formattedCondition,
		})
	}

	return writeJSON(c, http.StatusOK, responseList)
}

// POST /api/isu
// ISUを登録
func postIsu(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	useDefaultImage := false

	jiaIsuUUID := c.FormValue("jia_isu_uuid")
	isuName := c.FormValue("isu_name")
	fh, err := c.FormFile("image")
	if err != nil {
		if !errors.Is(err, http.ErrMissingFile) {
			return c.String(http.StatusBadRequest, "bad format: icon")
		}
		useDefaultImage = true
	}

	var image []byte

	if useDefaultImage {
		image, err = ioutil.ReadFile(defaultIconFilePath)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	} else {
		file, err := fh.Open()
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		defer file.Close()

		image, err = ioutil.ReadAll(file)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	tx, err := db.Beginx()
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	insertRes, err := tx.Exec("INSERT INTO `isu`"+
		"	(`jia_isu_uuid`, `name`, `image`, `jia_user_id`) VALUES (?, ?, ?, ?)",
		jiaIsuUUID, isuName, image, jiaUserID)
	if err != nil {
		mysqlErr, ok := err.(*mysql.MySQLError)

		if ok && mysqlErr.Number == uint16(mysqlErrNumDuplicateEntry) {
			return c.String(http.StatusConflict, "duplicated: isu")
		}

		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	isuID, err := insertRes.LastInsertId()
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	// Commit the parent row before asking JIA to activate the ISU. Activation can
	// immediately post conditions; holding this transaction would otherwise
	// deadlock with the condition table's foreign-key lock on isu.
	if err = tx.Commit(); err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	// activate 中に condition が来る前に、同じプロセスでは存在確認 SELECT を省略する。
	rememberIsuUUID(jiaIsuUUID)

	targetURL := getJIAServiceURL(db) + "/api/activate"
	body := JIAServiceRequest{postIsuConditionTargetBaseURL, jiaIsuUUID}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	reqJIA, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	reqJIA.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(reqJIA)
	if err != nil {
		c.Logger().Errorf("failed to request to JIAService: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer res.Body.Close()

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	if res.StatusCode != http.StatusAccepted {
		c.Logger().Errorf("JIAService returned error: status code %v, message: %v", res.StatusCode, string(resBody))
		return c.String(res.StatusCode, "JIAService returned error")
	}

	var isuFromJIA IsuFromJIA
	err = json.Unmarshal(resBody, &isuFromJIA)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, err = db.Exec("UPDATE `isu` SET `character` = ? WHERE  `jia_isu_uuid` = ?", isuFromJIA.Character, jiaIsuUUID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	meta := isuMeta{
		ID:         int(isuID),
		JIAIsuUUID: jiaIsuUUID,
		Name:       isuName,
		Character:  isuFromJIA.Character,
		JIAUserID:  jiaUserID,
	}
	rememberIsuMeta(meta)
	isu := Isu{
		ID:         meta.ID,
		JIAIsuUUID: meta.JIAIsuUUID,
		Name:       meta.Name,
		Character:  meta.Character,
		JIAUserID:  meta.JIAUserID,
		Image:      image,
	}

	if _, err := cacheIsuIcon(jiaUserID, jiaIsuUUID, image); err != nil {
		c.Logger().Errorf("failed to cache icon: %v", err)
	}
	invalidateTrendCache()

	return writeJSON(c, http.StatusCreated, isu)
}

// GET /api/isu/:jia_isu_uuid
// ISUの情報を取得
func getIsuID(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	jiaIsuUUID := c.Param("jia_isu_uuid")

	meta, ok, err := ownedIsuMeta(jiaIsuUUID, jiaUserID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if !ok {
		return c.String(http.StatusNotFound, "not found: isu")
	}

	return writeJSON(c, http.StatusOK, Isu{
		ID:         meta.ID,
		JIAIsuUUID: meta.JIAIsuUUID,
		Name:       meta.Name,
		Character:  meta.Character,
	})
}

// GET /api/isu/:jia_isu_uuid/icon
// ISUのアイコンを取得
func getIsuIcon(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	jiaIsuUUID := c.Param("jia_isu_uuid")
	if serveCachedIsuIcon(c, jiaUserID, jiaIsuUUID) {
		return c.NoContent(http.StatusOK)
	}

	var image []byte
	err = db.Get(&image, "SELECT `image` FROM `isu` WHERE `jia_user_id` = ? AND `jia_isu_uuid` = ?",
		jiaUserID, jiaIsuUUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "not found: isu")
		}

		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if _, err := cacheIsuIcon(jiaUserID, jiaIsuUUID, image); err != nil {
		c.Logger().Errorf("failed to cache icon: %v", err)
		return c.Blob(http.StatusOK, "", image)
	}
	if serveCachedIsuIcon(c, jiaUserID, jiaIsuUUID) {
		return c.NoContent(http.StatusOK)
	}

	return c.Blob(http.StatusOK, "", image)
}

// GET /api/isu/:jia_isu_uuid/graph
// ISUのコンディショングラフ描画のための情報を取得
func getIsuGraph(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	jiaIsuUUID := c.Param("jia_isu_uuid")
	datetimeStr := c.QueryParam("datetime")
	if datetimeStr == "" {
		return c.String(http.StatusBadRequest, "missing: datetime")
	}
	datetimeInt64, err := strconv.ParseInt(datetimeStr, 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "bad format: datetime")
	}
	date := time.Unix(datetimeInt64, 0).Truncate(time.Hour)
	cacheKey := graphCacheKey{jiaUserID: jiaUserID, jiaIsuUUID: jiaIsuUUID, startUnix: date.Unix()}
	if body, ok := graphCacheGet(cacheKey); ok {
		return writeJSONBytes(c, http.StatusOK, body)
	}

	_, ok, err := ownedIsuMeta(jiaIsuUUID, jiaUserID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if !ok {
		return c.String(http.StatusNotFound, "not found: isu")
	}

	res, err := generateIsuGraphResponse(db, jiaIsuUUID, date)
	if err != nil {
		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	body, err := marshalPrettyJSON(res)
	if err != nil {
		return err
	}
	graphCacheSet(cacheKey, body)
	return writeJSONBytes(c, http.StatusOK, body)
}

// グラフのデータ点を一日分生成
func generateIsuGraphResponse(q sqlx.Queryer, jiaIsuUUID string, graphDate time.Time) ([]GraphResponse, error) {
	endTime := graphDate.Add(24 * time.Hour)
	dataPoints := []GraphDataPointWithInfo{}
	conditionsInThisHour := []IsuCondition{}
	timestampsInThisHour := []int64{}
	var startTimeInThisHour time.Time
	var condition IsuCondition

	rows, err := q.Queryx("SELECT `timestamp`, `is_sitting`, `condition` FROM `isu_condition` WHERE `jia_isu_uuid` = ? AND `timestamp` >= ? AND `timestamp` < ? ORDER BY `timestamp` ASC", jiaIsuUUID, graphDate, endTime)
	if err != nil {
		return nil, fmt.Errorf("db error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.StructScan(&condition)
		if err != nil {
			return nil, err
		}

		truncatedConditionTime := condition.Timestamp.Truncate(time.Hour)
		if truncatedConditionTime != startTimeInThisHour {
			if len(conditionsInThisHour) > 0 {
				data, err := calculateGraphDataPoint(conditionsInThisHour)
				if err != nil {
					return nil, err
				}

				dataPoints = append(dataPoints,
					GraphDataPointWithInfo{
						JIAIsuUUID:          jiaIsuUUID,
						StartAt:             startTimeInThisHour,
						Data:                data,
						ConditionTimestamps: timestampsInThisHour})
			}

			startTimeInThisHour = truncatedConditionTime
			conditionsInThisHour = []IsuCondition{}
			timestampsInThisHour = []int64{}
		}
		conditionsInThisHour = append(conditionsInThisHour, condition)
		timestampsInThisHour = append(timestampsInThisHour, condition.Timestamp.Unix())
	}

	if len(conditionsInThisHour) > 0 {
		data, err := calculateGraphDataPoint(conditionsInThisHour)
		if err != nil {
			return nil, err
		}

		dataPoints = append(dataPoints,
			GraphDataPointWithInfo{
				JIAIsuUUID:          jiaIsuUUID,
				StartAt:             startTimeInThisHour,
				Data:                data,
				ConditionTimestamps: timestampsInThisHour})
	}

	startIndex := len(dataPoints)
	endNextIndex := len(dataPoints)
	for i, graph := range dataPoints {
		if startIndex == len(dataPoints) && !graph.StartAt.Before(graphDate) {
			startIndex = i
		}
		if endNextIndex == len(dataPoints) && graph.StartAt.After(endTime) {
			endNextIndex = i
		}
	}

	filteredDataPoints := []GraphDataPointWithInfo{}
	if startIndex < endNextIndex {
		filteredDataPoints = dataPoints[startIndex:endNextIndex]
	}

	responseList := []GraphResponse{}
	index := 0
	thisTime := graphDate

	for thisTime.Before(graphDate.Add(time.Hour * 24)) {
		var data *GraphDataPoint
		timestamps := []int64{}

		if index < len(filteredDataPoints) {
			dataWithInfo := filteredDataPoints[index]

			if dataWithInfo.StartAt.Equal(thisTime) {
				data = &dataWithInfo.Data
				timestamps = dataWithInfo.ConditionTimestamps
				index++
			}
		}

		resp := GraphResponse{
			StartAt:             thisTime.Unix(),
			EndAt:               thisTime.Add(time.Hour).Unix(),
			Data:                data,
			ConditionTimestamps: timestamps,
		}
		responseList = append(responseList, resp)

		thisTime = thisTime.Add(time.Hour)
	}

	return responseList, nil
}

// 複数のISUのコンディションからグラフの一つのデータ点を計算
func calculateGraphDataPoint(isuConditions []IsuCondition) (GraphDataPoint, error) {
	var isBrokenCount, isDirtyCount, isOverweightCount, sittingCount, rawScore int
	for i := range isuConditions {
		condition := &isuConditions[i]
		isDirty, isOverweight, isBroken, ok := parseConditionFlags(condition.Condition)
		if !ok {
			return GraphDataPoint{}, fmt.Errorf("invalid condition format")
		}

		badConditionsCount := 0
		if isDirty {
			isDirtyCount++
			badConditionsCount++
		}
		if isOverweight {
			isOverweightCount++
			badConditionsCount++
		}
		if isBroken {
			isBrokenCount++
			badConditionsCount++
		}

		if badConditionsCount >= 3 {
			rawScore += scoreConditionLevelCritical
		} else if badConditionsCount >= 1 {
			rawScore += scoreConditionLevelWarning
		} else {
			rawScore += scoreConditionLevelInfo
		}
		if condition.IsSitting {
			sittingCount++
		}
	}

	n := len(isuConditions)
	return GraphDataPoint{
		Score: rawScore * 100 / 3 / n,
		Percentage: ConditionsPercentage{
			Sitting:      sittingCount * 100 / n,
			IsBroken:     isBrokenCount * 100 / n,
			IsOverweight: isOverweightCount * 100 / n,
			IsDirty:      isDirtyCount * 100 / n,
		},
	}, nil
}

// GET /api/condition/:jia_isu_uuid
// ISUのコンディションを取得
func getIsuConditions(c echo.Context) error {
	jiaUserID, errStatusCode, err := getUserIDFromSession(c)
	if err != nil {
		if errStatusCode == http.StatusUnauthorized {
			return c.String(http.StatusUnauthorized, "you are not signed in")
		}

		c.Logger().Error(err)
		return c.NoContent(http.StatusInternalServerError)
	}

	jiaIsuUUID := c.Param("jia_isu_uuid")
	if jiaIsuUUID == "" {
		return c.String(http.StatusBadRequest, "missing: jia_isu_uuid")
	}

	endTimeInt64, err := strconv.ParseInt(c.QueryParam("end_time"), 10, 64)
	if err != nil {
		return c.String(http.StatusBadRequest, "bad format: end_time")
	}
	endTime := time.Unix(endTimeInt64, 0)
	conditionLevelCSV := c.QueryParam("condition_level")
	if conditionLevelCSV == "" {
		return c.String(http.StatusBadRequest, "missing: condition_level")
	}
	conditionLevel := map[string]interface{}{}
	for _, level := range strings.Split(conditionLevelCSV, ",") {
		conditionLevel[level] = struct{}{}
	}

	startTimeStr := c.QueryParam("start_time")
	var startTime time.Time
	if startTimeStr != "" {
		startTimeInt64, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			return c.String(http.StatusBadRequest, "bad format: start_time")
		}
		startTime = time.Unix(startTimeInt64, 0)
	}

	meta, ok, err := ownedIsuMeta(jiaIsuUUID, jiaUserID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if !ok {
		return c.String(http.StatusNotFound, "not found: isu")
	}
	isuName := meta.Name

	conditionsResponse, err := getIsuConditionsFromDB(db, jiaIsuUUID, endTime, conditionLevel, startTime, conditionLimit, isuName)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	return writeJSON(c, http.StatusOK, conditionsResponse)
}

// ISUのコンディションをDBから取得
func getIsuConditionsFromDBLegacy(db *sqlx.DB, jiaIsuUUID string, endTime time.Time, conditionLevel map[string]interface{}, startTime time.Time,
	limit int, isuName string) ([]*GetIsuConditionResponse, error) {

	conditions := []IsuCondition{}
	var err error

	if startTime.IsZero() {
		err = db.Select(&conditions,
			"SELECT * FROM `isu_condition` WHERE `jia_isu_uuid` = ?"+
				"	AND `timestamp` < ?"+
				"	ORDER BY `timestamp` DESC",
			jiaIsuUUID, endTime,
		)
	} else {
		err = db.Select(&conditions,
			"SELECT * FROM `isu_condition` WHERE `jia_isu_uuid` = ?"+
				"	AND `timestamp` < ?"+
				"	AND ? <= `timestamp`"+
				"	ORDER BY `timestamp` DESC",
			jiaIsuUUID, endTime, startTime,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("db error: %v", err)
	}

	conditionsResponse := []*GetIsuConditionResponse{}
	for _, c := range conditions {
		cLevel, err := calculateConditionLevel(c.Condition)
		if err != nil {
			continue
		}

		if _, ok := conditionLevel[cLevel]; ok {
			data := GetIsuConditionResponse{
				JIAIsuUUID:     c.JIAIsuUUID,
				IsuName:        isuName,
				Timestamp:      c.Timestamp.Unix(),
				IsSitting:      c.IsSitting,
				Condition:      c.Condition,
				ConditionLevel: cLevel,
				Message:        c.Message,
			}
			conditionsResponse = append(conditionsResponse, &data)
		}
	}

	if len(conditionsResponse) > limit {
		conditionsResponse = conditionsResponse[:limit]
	}

	return conditionsResponse, nil
}

func getIsuConditionsFromDB(db *sqlx.DB, jiaIsuUUID string, endTime time.Time, conditionLevel map[string]interface{}, startTime time.Time,
	limit int, isuName string) ([]*GetIsuConditionResponse, error) {
	// condition level は文字列から算出するため SQL で絞れない。全件を転送せず、
	// 時刻・id の keyset pagination で取得し、必要件数に達したら停止する。
	const batchSize = 64
	conditionsResponse := make([]*GetIsuConditionResponse, 0, limit)
	var cursorTimestamp time.Time
	var cursorID int
	hasCursor := false

	for {
		query := "SELECT `id`, `timestamp`, `is_sitting`, `condition`, `message` FROM `isu_condition` WHERE `jia_isu_uuid` = ? AND `timestamp` < ?"
		args := []interface{}{jiaIsuUUID, endTime}
		if !startTime.IsZero() {
			query += " AND `timestamp` >= ?"
			args = append(args, startTime)
		}
		if hasCursor {
			query += " AND (`timestamp` < ? OR (`timestamp` = ? AND `id` < ?))"
			args = append(args, cursorTimestamp, cursorTimestamp, cursorID)
		}
		query += " ORDER BY `timestamp` DESC, `id` DESC LIMIT ?"
		args = append(args, batchSize)

		conditions := []IsuCondition{}
		if err := db.Select(&conditions, query, args...); err != nil {
			return nil, fmt.Errorf("db error: %v", err)
		}
		for _, condition := range conditions {
			level, err := calculateConditionLevel(condition.Condition)
			if err != nil {
				continue
			}
			if _, ok := conditionLevel[level]; !ok {
				continue
			}
			conditionsResponse = append(conditionsResponse, &GetIsuConditionResponse{
				JIAIsuUUID:     jiaIsuUUID,
				IsuName:        isuName,
				Timestamp:      condition.Timestamp.Unix(),
				IsSitting:      condition.IsSitting,
				Condition:      condition.Condition,
				ConditionLevel: level,
				Message:        condition.Message,
			})
			if len(conditionsResponse) == limit {
				return conditionsResponse, nil
			}
		}
		if len(conditions) < batchSize {
			return conditionsResponse, nil
		}
		lastCondition := conditions[len(conditions)-1]
		cursorTimestamp = lastCondition.Timestamp
		cursorID = lastCondition.ID
		hasCursor = true
	}
}

// ISUのコンディションの文字列からコンディションレベルを計算
func calculateConditionLevel(condition string) (string, error) {
	var conditionLevel string

	warnCount := strings.Count(condition, "=true")
	switch warnCount {
	case 0:
		conditionLevel = conditionLevelInfo
	case 1, 2:
		conditionLevel = conditionLevelWarning
	case 3:
		conditionLevel = conditionLevelCritical
	default:
		return "", fmt.Errorf("unexpected warn count")
	}

	return conditionLevel, nil
}

// GET /api/trend
// ISUの性格毎の最新のコンディション情報
func getTrend(c echo.Context) error {
	// キャッシュミス時のDB再計算を1リクエストに限定する。
	trendCache.Lock()
	defer trendCache.Unlock()
	if trendCache.valid {
		res := trendCache.response
		return writeJSON(c, http.StatusOK, res)
	}

	characterList := []Isu{}
	err := db.Select(&characterList, "SELECT `character` FROM `isu` GROUP BY `character`")
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	conditionRows := []TrendConditionRow{}
	err = db.Select(&conditionRows, `
		SELECT isu.id, isu.character, latest.timestamp, latest.condition
		FROM isu
		JOIN isu_condition AS latest ON latest.id = (
			SELECT latest_condition.id
			FROM isu_condition AS latest_condition
			WHERE latest_condition.jia_isu_uuid = isu.jia_isu_uuid
			ORDER BY latest_condition.timestamp DESC, latest_condition.id DESC
			LIMIT 1
		)
		ORDER BY isu.character, latest.timestamp DESC, latest.id DESC`)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	info := map[string][]*TrendCondition{}
	warning := map[string][]*TrendCondition{}
	critical := map[string][]*TrendCondition{}
	for _, character := range characterList {
		info[character.Character] = []*TrendCondition{}
		warning[character.Character] = []*TrendCondition{}
		critical[character.Character] = []*TrendCondition{}
	}
	for _, row := range conditionRows {
		conditionLevel, err := calculateConditionLevel(row.Condition)
		if err != nil {
			c.Logger().Error(err)
			return c.NoContent(http.StatusInternalServerError)
		}
		trendCondition := &TrendCondition{ID: row.ID, Timestamp: row.Timestamp.Unix()}
		switch conditionLevel {
		case conditionLevelInfo:
			info[row.Character] = append(info[row.Character], trendCondition)
		case conditionLevelWarning:
			warning[row.Character] = append(warning[row.Character], trendCondition)
		case conditionLevelCritical:
			critical[row.Character] = append(critical[row.Character], trendCondition)
		}
	}

	res := make([]TrendResponse, 0, len(characterList))
	for _, character := range characterList {
		res = append(res, TrendResponse{
			Character: character.Character,
			Info:      info[character.Character],
			Warning:   warning[character.Character],
			Critical:  critical[character.Character],
		})
	}
	trendCache.response = res
	trendCache.valid = true

	return writeJSON(c, http.StatusOK, res)
}

func noteDroppedIsuCondition() {
	atomic.AddUint64(&dropConditionPending, 1)
	atomic.AddUint64(&dropConditionTotal, 1)
}

func startDropConditionLogFlusher() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var lastTotal uint64
		idle := time.Duration(0)
		for range ticker.C {
			total := atomic.LoadUint64(&dropConditionTotal)
			if total != lastTotal {
				lastTotal = total
				idle = 0
				continue
			}
			idle += time.Second
			if idle < dropConditionLogIdle {
				continue
			}
			n := atomic.SwapUint64(&dropConditionPending, 0)
			if n == 0 {
				continue
			}
			if dropConditionLogger != nil {
				dropConditionLogger.Warnf("drop post isu condition request: flushed=%d total=%d", n, total)
			}
			idle = 0
		}
	}()
}

// POST /api/condition/:jia_isu_uuid
// ISUからのコンディションを受け取る
func postIsuConditionLegacy(c echo.Context) error {
	// TODO: 一定割合リクエストを落としてしのぐようにしたが、本来は全量さばけるようにすべき
	dropProbability := 0.9
	if rand.Float64() <= dropProbability {
		noteDroppedIsuCondition()
		return c.NoContent(http.StatusAccepted)
	}

	jiaIsuUUID := c.Param("jia_isu_uuid")
	if jiaIsuUUID == "" {
		return c.String(http.StatusBadRequest, "missing: jia_isu_uuid")
	}

	req := []PostIsuConditionRequest{}
	err := c.Bind(&req)
	if err != nil {
		return c.String(http.StatusBadRequest, "bad request body")
	} else if len(req) == 0 {
		return c.String(http.StatusBadRequest, "bad request body")
	}

	tx, err := db.Beginx()
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer tx.Rollback()

	var count int
	err = tx.Get(&count, "SELECT COUNT(*) FROM `isu` WHERE `jia_isu_uuid` = ?", jiaIsuUUID)
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if count == 0 {
		return c.String(http.StatusNotFound, "not found: isu")
	}

	for _, cond := range req {
		timestamp := time.Unix(cond.Timestamp, 0)

		if !isValidConditionFormat(cond.Condition) {
			return c.String(http.StatusBadRequest, "bad request body")
		}

		_, err = tx.Exec(
			"INSERT INTO `isu_condition`"+
				"	(`jia_isu_uuid`, `timestamp`, `is_sitting`, `condition`, `message`)"+
				"	VALUES (?, ?, ?, ?, ?)",
			jiaIsuUUID, timestamp, cond.IsSitting, cond.Condition, cond.Message)
		if err != nil {
			c.Logger().Errorf("db error: %v", err)
			return c.NoContent(http.StatusInternalServerError)
		}

	}

	err = tx.Commit()
	if err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	invalidateTrendCache()

	return c.NoContent(http.StatusAccepted)
}

func postIsuCondition(c echo.Context) error {
	// 既存の暫定的な drop 挙動は変更しない。DB 書き込みだけを一括化する。
	dropProbability := 0.9
	if rand.Float64() <= dropProbability {
		noteDroppedIsuCondition()
		return c.NoContent(http.StatusAccepted)
	}

	requestStart := time.Now()
	jiaIsuUUID := c.Param("jia_isu_uuid")
	if jiaIsuUUID == "" {
		return c.String(http.StatusBadRequest, "missing: jia_isu_uuid")
	}
	req := []PostIsuConditionRequest{}
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "bad request body")
	} else if len(req) == 0 {
		return c.String(http.StatusBadRequest, "bad request body")
	}

	args := make([]interface{}, 0, len(req)*5)
	for _, cond := range req {
		if !isValidConditionFormat(cond.Condition) {
			return c.String(http.StatusBadRequest, "bad request body")
		}
		args = append(args, jiaIsuUUID, time.Unix(cond.Timestamp, 0), cond.IsSitting, cond.Condition, cond.Message)
	}

	// 正の uuid / メタデータキャッシュに無ければ 1 回だけ確認する。404 はキャッシュしない。
	if _, ok := isuUUIDCache.Load(jiaIsuUUID); !ok {
		_, exists, err := loadIsuMetaByUUID(jiaIsuUUID)
		if err != nil {
			c.Logger().Errorf("db error: %v", err)
			return c.NoContent(http.StatusInternalServerError)
		}
		if !exists {
			return c.String(http.StatusNotFound, "not found: isu")
		}
	}

	// 短い待ち窓で他リクエストの行とまとめて 1 INSERT する（#240）。
	if err := globalConditionBatcher.enqueue(args, len(req), requestStart); err != nil {
		c.Logger().Errorf("db error: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.NoContent(http.StatusAccepted)
}

// ISUのコンディションの文字列がcsv形式になっているか検証
func isValidConditionFormat(conditionStr string) bool {
	_, _, _, ok := parseConditionFlags(conditionStr)
	return ok
}

// parseConditionFlags は is_dirty / is_overweight / is_broken の固定順CSVを
// 割り当てなしで検証し、各フラグが true かを返す。
func parseConditionFlags(conditionStr string) (isDirty, isOverweight, isBroken, ok bool) {
	keys := [...]string{"is_dirty=", "is_overweight=", "is_broken="}
	var flags [3]bool
	idx := 0
	for i, key := range keys {
		if idx > len(conditionStr) || !strings.HasPrefix(conditionStr[idx:], key) {
			return false, false, false, false
		}
		idx += len(key)
		if strings.HasPrefix(conditionStr[idx:], "true") {
			flags[i] = true
			idx += len("true")
		} else if strings.HasPrefix(conditionStr[idx:], "false") {
			idx += len("false")
		} else {
			return false, false, false, false
		}
		if i < len(keys)-1 {
			if idx >= len(conditionStr) || conditionStr[idx] != ',' {
				return false, false, false, false
			}
			idx++
		}
	}
	if idx != len(conditionStr) {
		return false, false, false, false
	}
	return flags[0], flags[1], flags[2], true
}

func getIndex(c echo.Context) error {
	return c.File(frontendContentsPath + "/index.html")
}
