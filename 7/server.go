package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	cookiePrefix   = "fa_"
	jwtCookieName  = "fa_jwt"
	maxFioLen      = 150
	maxBioLen      = 5000
	maxPhoneLen    = 30
	maxEmailLen    = 255
	minLangID      = 1
	maxLangID      = 12
	cookieMaxAge   = 365 * 24 * 3600
	loginLen       = 10
	passwordLen    = 10
	jwtExpireHours = 24 * 7
	alnum          = "abcdefghijklmnopqrstuvwxyz0123456789"
)

var (
	db          *sql.DB
	fioRegex    = regexp.MustCompile(`^[\p{L}\s\-]+$`)
	phoneRegex  = regexp.MustCompile(`^[\d\s+\-()+]+$`)
	emailRegex  = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	dateRegex   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	genderRegex = regexp.MustCompile(`^(male|female)$`)
	langIDRegex = regexp.MustCompile(`^(1[0-2]|[1-9])$`)
)

type formData struct {
	Fio       string   `json:"fio"`
	Phone     string   `json:"phone"`
	Email     string   `json:"email"`
	Birthdate string   `json:"birthdate"`
	Gender    string   `json:"gender"`
	Languages []string `json:"languages"`
	Biography string   `json:"bio"`
	Agreement bool     `json:"agreement"`
}

type createResponse struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type errorResponse struct {
	Errors map[string]string `json:"errors"`
}

func getDSN() string {
	user := getEnv("MYSQL_USER", "root")
	pass := getEnv("MYSQL_PASSWORD", "")
	host := getEnv("MYSQL_HOST", "localhost")
	port := getEnv("MYSQL_PORT", "3306")
	dbname := getEnv("MYSQL_DATABASE", "test")
	if pass != "" {
		return user + ":" + pass + "@tcp(" + host + ":" + port + ")/" + dbname + "?charset=utf8mb4&parseTime=true"
	}
	return user + "@tcp(" + host + ":" + port + ")/" + dbname + "?charset=utf8mb4&parseTime=true"
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBasePath() string {
	if os.Getenv("GATEWAY_INTERFACE") != "" || os.Getenv("REQUEST_METHOD") != "" {
		if s := os.Getenv("SCRIPT_NAME"); s != "" {
			s = strings.TrimSuffix(s, "/")
			return s
		}
	}
	if b := os.Getenv("BASE_PATH"); b != "" {
		if !strings.HasPrefix(b, "/") {
			b = "/" + b
		}
		return strings.TrimSuffix(b, "/")
	}
	return "/7"
}

func cookieEncode(s string) string {
	if s == "" {
		return ""
	}
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func cookieDecode(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return string(b)
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = alnum[int(b[i])%len(alnum)]
	}
	return string(b)
}

func hashPassword(password, salt string) string {
	h := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(h[:])
}

func jwtSecret() []byte {
	return []byte(getEnv("JWT_SECRET", "formapp7-secret"))
}

func jwtCreate(appID int64) string {
	header := base64.URLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.URLEncoding.EncodeToString([]byte(`{"app_id":` + strconv.FormatInt(appID, 10) + `,"exp":` + strconv.FormatInt(time.Now().Add(jwtExpireHours*time.Hour).Unix(), 10) + `}`))
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(unsigned))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig
}

func jwtVerify(token string) (appID int64, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, false
	}
	mac := hmac.New(sha256.New, jwtSecret())
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	if sig != parts[2] {
		return 0, false
	}
	payload, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}
	var v struct {
		AppID int64 `json:"app_id"`
		Exp   int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &v) != nil {
		return 0, false
	}
	if time.Now().Unix() > v.Exp {
		return 0, false
	}
	return v.AppID, true
}

func getJWT(r *http.Request) string {
	for _, c := range r.Cookies() {
		if c.Name == jwtCookieName {
			return c.Value
		}
	}
	return ""
}

func validateRegex(f formData) map[string]string {
	errs := make(map[string]string)
	if f.Fio == "" {
		errs["fio"] = "Поле обязательно."
	} else if len(f.Fio) > maxFioLen {
		errs["fio"] = "Не более 150 символов."
	} else if !fioRegex.MatchString(f.Fio) {
		errs["fio"] = "Только буквы, пробелы и дефис."
	}
	if f.Phone == "" {
		errs["phone"] = "Поле обязательно."
	} else if len(f.Phone) > maxPhoneLen {
		errs["phone"] = "Не более 30 символов."
	} else if !phoneRegex.MatchString(f.Phone) {
		errs["phone"] = "Только цифры и + - ( )."
	}
	if f.Email == "" {
		errs["email"] = "Поле обязательно."
	} else if len(f.Email) > maxEmailLen {
		errs["email"] = "Не более 255 символов."
	} else if !emailRegex.MatchString(f.Email) {
		errs["email"] = "Недопустимый формат e-mail."
	}
	if f.Birthdate == "" {
		errs["birthdate"] = "Поле обязательно."
	} else if !dateRegex.MatchString(f.Birthdate) {
		errs["birthdate"] = "Формат ГГГГ-ММ-ДД."
	} else if _, e := parseDate(f.Birthdate); e != nil {
		errs["birthdate"] = "Недопустимая дата."
	}
	if f.Gender == "" {
		errs["gender"] = "Выберите пол."
	} else if !genderRegex.MatchString(f.Gender) {
		errs["gender"] = "Недопустимое значение."
	}
	if len(f.Languages) == 0 {
		errs["languages"] = "Выберите хотя бы один язык."
	} else {
		for _, s := range f.Languages {
			if !langIDRegex.MatchString(s) {
				errs["languages"] = "Недопустимый язык."
				break
			}
			id, _ := strconv.Atoi(s)
			if id < minLangID || id > maxLangID {
				errs["languages"] = "Недопустимый язык."
				break
			}
		}
	}
	if len(f.Biography) > maxBioLen {
		errs["bio"] = "Не более " + strconv.Itoa(maxBioLen) + " символов."
	}
	if !f.Agreement {
		errs["agreement"] = "Отметьте ознакомление с контрактом."
	}
	return errs
}

func parseDate(s string) (string, error) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return "", strconv.ErrSyntax
	}
	y, _ := strconv.Atoi(s[:4])
	m, _ := strconv.Atoi(s[5:7])
	d, _ := strconv.Atoi(s[8:10])
	if y < 1900 || y > 2100 || m < 1 || m > 12 || d < 1 || d > 31 {
		return "", strconv.ErrSyntax
	}
	return s, nil
}

func saveApplication(f formData) (id int64, login, password string, err error) {
	login = generateRandomString(loginLen)
	password = generateRandomString(passwordLen)
	salt := generateRandomString(16)
	hash := hashPassword(password, salt)
	for i := 0; i < 10; i++ {
		tx, e := db.Begin()
		if e != nil {
			return 0, "", "", e
		}
		defer tx.Rollback()
		stmt, e := tx.Prepare("INSERT INTO application (login, password_hash, salt, fio, phone, email, birthdate, gender, biography, contract_agreed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if e != nil {
			return 0, "", "", e
		}
		agreed := 0
		if f.Agreement {
			agreed = 1
		}
		res, e := stmt.Exec(login, hash, salt, f.Fio, f.Phone, f.Email, f.Birthdate, f.Gender, f.Biography, agreed)
		stmt.Close()
		if e != nil {
			if strings.Contains(e.Error(), "Duplicate") {
				login = generateRandomString(loginLen)
				continue
			}
			return 0, "", "", e
		}
		id, e = res.LastInsertId()
		if e != nil {
			return 0, "", "", e
		}
		linkStmt, e := tx.Prepare("INSERT INTO application_language (application_id, language_id) VALUES (?, ?)")
		if e != nil {
			return 0, "", "", e
		}
		for _, langIDStr := range f.Languages {
			linkStmt.Exec(id, langIDStr)
		}
		linkStmt.Close()
		return id, login, password, tx.Commit()
	}
	return 0, "", "", sql.ErrNoRows
}

func updateApplication(appID int64, f formData) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	agreed := 0
	if f.Agreement {
		agreed = 1
	}
	_, err = tx.Exec("UPDATE application SET fio=?, phone=?, email=?, birthdate=?, gender=?, biography=?, contract_agreed=? WHERE id=?", f.Fio, f.Phone, f.Email, f.Birthdate, f.Gender, f.Biography, agreed, appID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM application_language WHERE application_id=?", appID)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO application_language (application_id, language_id) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, langIDStr := range f.Languages {
		stmt.Exec(appID, langIDStr)
	}
	return tx.Commit()
}

func loadApplication(appID int64) *formData {
	var f formData
	var bio sql.NullString
	var agreed int
	var bd time.Time
	err := db.QueryRow("SELECT fio, phone, email, birthdate, gender, biography, contract_agreed FROM application WHERE id=?", appID).Scan(
		&f.Fio, &f.Phone, &f.Email, &bd, &f.Gender, &bio, &agreed,
	)
	if err != nil {
		return nil
	}
	f.Birthdate = bd.Format("2006-01-02")
	if bio.Valid {
		f.Biography = bio.String
	}
	f.Agreement = agreed != 0
	rows, err := db.Query("SELECT language_id FROM application_language WHERE application_id=?", appID)
	if err != nil {
		return &f
	}
	defer rows.Close()
	for rows.Next() {
		var lid int
		rows.Scan(&lid)
		f.Languages = append(f.Languages, strconv.Itoa(lid))
	}
	return &f
}

func findApplicationByLogin(login string) (appID int64, hash, salt string, ok bool) {
	err := db.QueryRow("SELECT id, password_hash, salt FROM application WHERE login=?", login).Scan(&appID, &hash, &salt)
	return appID, hash, salt, err == nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func apiCreate(w http.ResponseWriter, r *http.Request) {
	var f formData
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Errors: map[string]string{"_": "Неверный JSON."}})
		return
	}
	f.Fio = strings.TrimSpace(f.Fio)
	f.Phone = strings.TrimSpace(f.Phone)
	f.Email = strings.TrimSpace(f.Email)
	f.Birthdate = strings.TrimSpace(f.Birthdate)
	f.Gender = strings.TrimSpace(f.Gender)
	f.Biography = strings.TrimSpace(f.Biography)
	errors := validateRegex(f)
	if len(errors) > 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Errors: errors})
		return
	}
	id, login, password, err := saveApplication(f)
	if err != nil {
		log.Printf("save application: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Errors: map[string]string{"_": "Ошибка сохранения."}})
		return
	}
	_ = id
	writeJSON(w, http.StatusOK, createResponse{
		Login:    login,
		Password: password,
	})
}

func apiUpdate(w http.ResponseWriter, r *http.Request, appID int64) {
	var f formData
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Errors: map[string]string{"_": "Неверный JSON."}})
		return
	}
	f.Fio = strings.TrimSpace(f.Fio)
	f.Phone = strings.TrimSpace(f.Phone)
	f.Email = strings.TrimSpace(f.Email)
	f.Birthdate = strings.TrimSpace(f.Birthdate)
	f.Gender = strings.TrimSpace(f.Gender)
	f.Biography = strings.TrimSpace(f.Biography)
	errors := validateRegex(f)
	if len(errors) > 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Errors: errors})
		return
	}
	if err := updateApplication(appID, f); err != nil {
		log.Printf("update application: %v", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Errors: map[string]string{"_": "Ошибка сохранения."}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func apiMe(w http.ResponseWriter, r *http.Request) {
	token := getJWT(r)
	appID, ok := jwtVerify(token)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Требуется авторизация."})
		return
	}
	f := loadApplication(appID)
	if f == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Анкета не найдена."})
		return
	}
	out := map[string]interface{}{
		"id":        appID,
		"fio":       f.Fio,
		"phone":     f.Phone,
		"email":     f.Email,
		"birthdate": f.Birthdate,
		"gender":    f.Gender,
		"languages": f.Languages,
		"bio":       f.Biography,
		"agreement": f.Agreement,
	}
	writeJSON(w, http.StatusOK, out)
}

func apiLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный JSON."})
			return
		}
	} else {
		r.ParseForm()
		req.Login = strings.TrimSpace(r.FormValue("login"))
		req.Password = r.FormValue("password")
	}
	if req.Login == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Введите логин и пароль."})
		return
	}
	appID, hash, salt, ok := findApplicationByLogin(req.Login)
	if !ok || hashPassword(req.Password, salt) != hash {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Неверный логин или пароль."})
		return
	}
	base := getBasePath()
	http.SetCookie(w, &http.Cookie{Name: jwtCookieName, Value: jwtCreate(appID), Path: base, MaxAge: int(jwtExpireHours * 3600)})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func setSessionErrorCookies(w http.ResponseWriter, form formData, errors map[string]string) {
	cookiePath := getBasePath()
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "from", Value: "error", Path: cookiePath})
	setFormCookies(w, form, 0)
	for k, v := range errors {
		if k == "_" {
			continue
		}
		http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "err_" + k, Value: cookieEncode(v), Path: cookiePath})
	}
}

func setFormCookies(w http.ResponseWriter, form formData, maxAge int) {
	cookiePath := getBasePath()
	opts := []struct{ name, val string }{
		{"fio", form.Fio}, {"phone", form.Phone}, {"email", form.Email}, {"birthdate", form.Birthdate},
		{"gender", form.Gender}, {"languages", strings.Join(form.Languages, ",")}, {"bio", form.Biography},
	}
	if form.Agreement {
		opts = append(opts, struct{ name, val string }{"agreement", "on"})
	} else {
		opts = append(opts, struct{ name, val string }{"agreement", ""})
	}
	for _, o := range opts {
		c := &http.Cookie{Name: cookiePrefix + o.name, Value: cookieEncode(o.val), Path: cookiePath}
		if maxAge > 0 {
			c.MaxAge = maxAge
		}
		http.SetCookie(w, c)
	}
}

func setSuccessMessageCookie(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "msg", Value: cookieEncode(msg), Path: getBasePath(), MaxAge: 60})
}

func redirectTo(w http.ResponseWriter, loc string) {
	w.Header().Set("Location", loc)
	w.WriteHeader(http.StatusSeeOther)
}

func fallbackResultPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	base := getBasePath()
	var msg string
	var errs []string
	for _, c := range r.Cookies() {
		if c.Name == cookiePrefix+"msg" && c.Value != "" {
			msg = cookieDecode(c.Value)
			break
		}
	}
	if msg == "" {
		for _, c := range r.Cookies() {
			if strings.HasPrefix(c.Name, cookiePrefix+"err_") && c.Value != "" {
				errs = append(errs, cookieDecode(c.Value))
			}
		}
	}
	backLink := base + "/"
	html := "<!DOCTYPE html><html><head><meta charset=\"UTF-8\"><title>Результат</title></head><body>"
	if msg != "" {
		html += "<p class=\"success\">" + template.HTMLEscapeString(msg) + "</p>"
	} else if len(errs) > 0 {
		html += "<p class=\"error\">Ошибки:</p><ul>"
		for _, e := range errs {
			html += "<li>" + template.HTMLEscapeString(e) + "</li>"
		}
		html += "</ul>"
	} else {
		html += "<p>Нет данных для отображения.</p>"
	}
	html += "<p><a href=\"" + template.HTMLEscapeString(backLink) + "\">Вернуться к форме</a></p></body></html>"
	w.Write([]byte(html))
}

func fallbackPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectTo(w, getBasePath()+"/")
		return
	}
	form := formData{
		Fio:       strings.TrimSpace(r.PostFormValue("fio")),
		Phone:     strings.TrimSpace(r.PostFormValue("phone")),
		Email:     strings.TrimSpace(r.PostFormValue("email")),
		Birthdate: strings.TrimSpace(r.PostFormValue("birthdate")),
		Gender:    strings.TrimSpace(r.PostFormValue("gender")),
		Biography: strings.TrimSpace(r.PostFormValue("bio")),
		Agreement: r.PostFormValue("agreement") == "on",
	}
	form.Languages = r.PostForm["languages"]
	errors := validateRegex(form)
	if len(errors) > 0 {
		setSessionErrorCookies(w, form, errors)
		redirectTo(w, getBasePath()+"/result?from=error")
		return
	}
	token := getJWT(r)
	appID, auth := jwtVerify(token)
	if auth {
		if err := updateApplication(appID, form); err != nil {
			log.Printf("update application: %v", err)
			setSessionErrorCookies(w, form, map[string]string{"_": "Ошибка сохранения."})
			redirectTo(w, getBasePath()+"/result?from=error")
			return
		}
		setSuccessMessageCookie(w, "Данные обновлены.")
		redirectTo(w, getBasePath()+"/result?msg=saved")
		return
	}
	id, login, password, err := saveApplication(form)
	if err != nil {
		log.Printf("save application: %v", err)
		setSessionErrorCookies(w, form, map[string]string{"_": "Ошибка сохранения."})
		redirectTo(w, getBasePath()+"/result?from=error")
		return
	}
	_ = id
	setFormCookies(w, form, cookieMaxAge)
	setSuccessMessageCookie(w, "Ваш логин: "+login+", пароль: "+password)
	redirectTo(w, getBasePath()+"/result?msg=success")
}

func main() {
	if os.Getenv("GATEWAY_INTERFACE") != "" || (os.Getenv("REQUEST_METHOD") != "" && os.Getenv("SCRIPT_NAME") != "") {
		runCGI()
		return
	}
	dsn := getDSN()
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("open db: ", err)
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		log.Fatal("ping db: ", err)
	}
	base := getBasePath()
	staticDir := "."
	if d := os.Getenv("STATIC_DIR"); d != "" {
		staticDir = d
	}
	fs := http.StripPrefix(base+"/", http.FileServer(http.Dir(staticDir)))
	http.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, base)
		path = strings.TrimPrefix(path, "/")
		if (path == "" || path == "index.html") && r.Method == http.MethodGet {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		if path == "login" && r.Method == http.MethodPost {
			apiLogin(w, r)
			return
		}
		if path == "login" && r.Method == http.MethodGet {
			http.Redirect(w, r, base+"/", http.StatusFound)
			return
		}
		if path == "me" && r.Method == http.MethodGet {
			apiMe(w, r)
			return
		}
		if path == "result" && r.Method == http.MethodGet {
			fallbackResultPage(w, r)
			return
		}
		if pathPrefix := "applications/"; strings.HasPrefix(path, pathPrefix) && r.Method == http.MethodPut {
			idStr := strings.TrimPrefix(path, pathPrefix)
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный id."})
				return
			}
			token := getJWT(r)
			appID, ok := jwtVerify(token)
			if !ok || appID != id {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Требуется авторизация."})
				return
			}
			apiUpdate(w, r, id)
			return
		}
		if r.Method == http.MethodPost && (path == "" || path == "applications") {
			ct := r.Header.Get("Content-Type")
			if strings.Contains(ct, "application/json") {
				apiCreate(w, r)
				return
			}
			fallbackPost(w, r)
			return
		}
		fs.ServeHTTP(w, r)
	})
	if base != "/" {
		http.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, base+"/", http.StatusFound)
		})
	}
	addr := getEnv("LISTEN_ADDR", ":8080")
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

type cgiResponseWriter struct {
	status int
	header http.Header
	body   bytes.Buffer
}

func (c *cgiResponseWriter) Header() http.Header {
	if c.header == nil {
		c.header = make(http.Header)
	}
	return c.header
}
func (c *cgiResponseWriter) WriteHeader(code int) { c.status = code }
func (c *cgiResponseWriter) Write(p []byte) (int, error) { return c.body.Write(p) }

func runCGI() {
	var err error
	db, err = sql.Open("mysql", getDSN())
	if err != nil {
		writeCGIErr("Ошибка БД")
		return
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		writeCGIErr("Ошибка БД")
		return
	}
	method := os.Getenv("REQUEST_METHOD")
	if method == "" {
		method = "GET"
	}
	base := getBasePath()
	requestURI := os.Getenv("REQUEST_URI")
	if requestURI == "" {
		requestURI = os.Getenv("SCRIPT_NAME") + os.Getenv("PATH_INFO")
	}
	if i := strings.Index(requestURI, "?"); i >= 0 {
		requestURI = requestURI[:i]
	}
	path := strings.TrimPrefix(requestURI, base)
	path = strings.TrimPrefix(path, "/")
	u := &url.URL{Path: base + "/"}
	if path != "" {
		u.Path = base + "/" + path
	}
	u.RawQuery = os.Getenv("QUERY_STRING")
	var body io.Reader
	if method == "POST" || method == "PUT" {
		if n := os.Getenv("CONTENT_LENGTH"); n != "" {
			if size, _ := strconv.Atoi(n); size > 0 && size < 1<<20 {
				body = io.LimitReader(os.Stdin, int64(size))
			}
		}
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		writeCGIErr("Ошибка запроса")
		return
	}
	if (method == "POST" || method == "PUT") && os.Getenv("CONTENT_TYPE") != "" {
		req.Header.Set("Content-Type", os.Getenv("CONTENT_TYPE"))
	}
	req.Header.Set("Cookie", os.Getenv("HTTP_COOKIE"))
	if auth := os.Getenv("HTTP_AUTHORIZATION"); auth != "" {
		req.Header.Set("Authorization", auth)
	} else if auth := os.Getenv("REDIRECT_HTTP_AUTHORIZATION"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := &cgiResponseWriter{status: http.StatusOK}
	serveRequest(w, req, path)
	out := os.Stdout
	out.Write([]byte("Status: " + strconv.Itoa(w.status) + " " + http.StatusText(w.status) + "\r\n"))
	for k, vv := range w.header {
		for _, v := range vv {
			out.Write([]byte(k + ": " + v + "\r\n"))
		}
	}
	out.Write([]byte("\r\n"))
	w.body.WriteTo(out)
}

func writeCGIErr(msg string) {
	os.Stdout.Write([]byte("Status: 500 Internal Server Error\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html><body><h1>" + msg + "</h1></body></html>"))
}

func serveRequest(w http.ResponseWriter, r *http.Request, path string) {
	base := getBasePath()
	staticDir := "."
	if d := os.Getenv("STATIC_DIR"); d != "" {
		staticDir = d
	}
	if (path == "" || path == "index.html") && r.Method == http.MethodGet {
		indexPath := filepath.Join(staticDir, "index.html")
		if os.Getenv("GATEWAY_INTERFACE") != "" {
			data, err := os.ReadFile(indexPath)
			if err == nil {
				scriptBase := base
				if scriptBase == "" {
					scriptBase = "/7"
				}
				escaped, _ := json.Marshal(scriptBase)
				repl := []byte("<script>window.API_BASE_PATH=" + string(escaped) + ";</script>")
				data = bytes.Replace(data, []byte("<!-- API_BASE_PATH -->"), repl, 1)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
		}
		http.ServeFile(w, r, indexPath)
		return
	}
	if path == "login" && r.Method == http.MethodPost {
		apiLogin(w, r)
		return
	}
	if path == "login" && r.Method == http.MethodGet {
		w.Header().Set("Location", base+"/")
		w.WriteHeader(http.StatusSeeOther)
		return
	}
	if path == "me" && r.Method == http.MethodGet {
		apiMe(w, r)
		return
	}
	if path == "result" && r.Method == http.MethodGet {
		fallbackResultPage(w, r)
		return
	}
	if pathPrefix := "applications/"; strings.HasPrefix(path, pathPrefix) && r.Method == http.MethodPut {
		idStr := strings.TrimPrefix(path, pathPrefix)
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Неверный id."})
			return
		}
		token := getJWT(r)
		appID, ok := jwtVerify(token)
		if !ok || appID != id {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Требуется авторизация."})
			return
		}
		apiUpdate(w, r, id)
		return
	}
	if r.Method == http.MethodPost && (path == "" || path == "applications") {
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			apiCreate(w, r)
			return
		}
		fallbackPost(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(staticDir, path))
}
