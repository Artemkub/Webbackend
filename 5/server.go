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
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	cookiePrefix     = "fa_"
	jwtCookieName    = "fa_jwt"
	maxFioLen        = 150
	maxBioLen        = 5000
	maxPhoneLen      = 30
	maxEmailLen      = 255
	minLangID        = 1
	maxLangID        = 12
	cookieMaxAgeYear = 365 * 24 * 3600
	loginLen         = 10
	passwordLen      = 10
	jwtExpireHours   = 24 * 7
	alnum            = "abcdefghijklmnopqrstuvwxyz0123456789"
)

var (
	db          *sql.DB
	formTpl     *template.Template
	loginTpl    *template.Template
	fioRegex    = regexp.MustCompile(`^[\p{L}\s\-]+$`)
	phoneRegex  = regexp.MustCompile(`^[\d\s+\-()+]+$`)
	emailRegex  = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	dateRegex   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	genderRegex = regexp.MustCompile(`^(male|female)$`)
	langIDRegex = regexp.MustCompile(`^(1[0-2]|[1-9])$`)
)

const formHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Анкета</title>
<style>
body{font-family:sans-serif;max-width:480px;margin:2rem auto;padding:0 1rem}
h1{margin-top:0}
.msg{margin-bottom:1rem;padding:.75rem;border-radius:6px}
.msg.ok{background:#efe;border:1px solid #0a0}
.field-error input,.field-error select,.field-error textarea{border-color:#c00;background:#fff8f8}
.err-msg{display:block;color:#c00;font-size:0.9em;margin-top:0.2rem}
label{display:block;margin-top:.75rem;margin-bottom:.25rem}
input[type="text"],input[type="tel"],input[type="email"],input[type="date"],select,textarea{width:100%;padding:.4rem;border:1px solid #ccc}
select[multiple]{min-height:120px}
textarea{min-height:80px;resize:vertical}
button{margin-top:1rem;padding:.5rem 1rem}
a{color:#06c}
</style>
</head>
<body>
<h1>Анкета</h1>
{{if .Message}}
<div class="msg ok">{{.Message}}</div>
{{end}}
{{if .Auth}}
<p>Логин: {{.CurrentLogin}} | <a href="{{.LoginPath}}">Войти снова</a></p>
{{else}}
<p><a href="{{.LoginPath}}">Войти</a></p>
{{end}}
<form action="" method="POST">
<div class="{{if .Errors.fio}}field-error{{end}}">
<label for="fio">ФИО:</label>
{{if .Errors.fio}}<span class="err-msg">{{.Errors.fio}}</span>{{end}}
<input type="text" id="fio" name="fio" maxlength="150" value="{{.Form.Fio}}">
</div>
<div class="{{if .Errors.phone}}field-error{{end}}">
<label for="phone">Телефон:</label>
{{if .Errors.phone}}<span class="err-msg">{{.Errors.phone}}</span>{{end}}
<input type="tel" id="phone" name="phone" value="{{.Form.Phone}}">
</div>
<div class="{{if .Errors.email}}field-error{{end}}">
<label for="email">E-mail:</label>
{{if .Errors.email}}<span class="err-msg">{{.Errors.email}}</span>{{end}}
<input type="email" id="email" name="email" value="{{.Form.Email}}">
</div>
<div class="{{if .Errors.birthdate}}field-error{{end}}">
<label for="birthdate">Дата рождения:</label>
{{if .Errors.birthdate}}<span class="err-msg">{{.Errors.birthdate}}</span>{{end}}
<input type="date" id="birthdate" name="birthdate" value="{{.Form.Birthdate}}">
</div>
<div class="{{if .Errors.gender}}field-error{{end}}">
<label>Пол:</label>
{{if .Errors.gender}}<span class="err-msg">{{.Errors.gender}}</span>{{end}}
<input type="radio" id="male" name="gender" value="male" {{if eq .Form.Gender "male"}}checked{{end}}>
<label for="male">Мужской</label>
<input type="radio" id="female" name="gender" value="female" {{if eq .Form.Gender "female"}}checked{{end}}>
<label for="female">Женский</label>
</div>
<div class="{{if .Errors.languages}}field-error{{end}}">
<label for="languages">Любимый язык программирования:</label>
{{if .Errors.languages}}<span class="err-msg">{{.Errors.languages}}</span>{{end}}
<select id="languages" name="languages" multiple size="6">
<option value="1" {{if selected .Form.Languages "1"}}selected{{end}}>Pascal</option>
<option value="2" {{if selected .Form.Languages "2"}}selected{{end}}>C</option>
<option value="3" {{if selected .Form.Languages "3"}}selected{{end}}>C++</option>
<option value="4" {{if selected .Form.Languages "4"}}selected{{end}}>JavaScript</option>
<option value="5" {{if selected .Form.Languages "5"}}selected{{end}}>PHP</option>
<option value="6" {{if selected .Form.Languages "6"}}selected{{end}}>Python</option>
<option value="7" {{if selected .Form.Languages "7"}}selected{{end}}>Java</option>
<option value="8" {{if selected .Form.Languages "8"}}selected{{end}}>Haskel</option>
<option value="9" {{if selected .Form.Languages "9"}}selected{{end}}>Clojure</option>
<option value="10" {{if selected .Form.Languages "10"}}selected{{end}}>Prolog</option>
<option value="11" {{if selected .Form.Languages "11"}}selected{{end}}>Scala</option>
<option value="12" {{if selected .Form.Languages "12"}}selected{{end}}>Go</option>
</select>
</div>
<div class="{{if .Errors.bio}}field-error{{end}}">
<label for="bio">Биография:</label>
{{if .Errors.bio}}<span class="err-msg">{{.Errors.bio}}</span>{{end}}
<textarea id="bio" name="bio" rows="5" cols="40">{{.Form.Biography}}</textarea>
</div>
<div class="{{if .Errors.agreement}}field-error{{end}}">
{{if .Errors.agreement}}<span class="err-msg">{{.Errors.agreement}}</span>{{end}}
<input type="checkbox" id="agreement" name="agreement" value="on" {{if .Form.Agreement}}checked{{end}}>
<label for="agreement">С контрактом ознакомлен(а)</label>
</div>
<br>
<button type="submit">Сохранить</button>
</form>
</body>
</html>`

const loginHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Вход</title>
<style>
body{font-family:sans-serif;max-width:360px;margin:2rem auto;padding:0 1rem}
h1{margin-top:0}
.msg.err{background:#fee;border:1px solid #c00;padding:.75rem;margin-bottom:1rem;border-radius:6px}
label{display:block;margin-top:.75rem;margin-bottom:.25rem}
input{width:100%;padding:.4rem}
button{margin-top:1rem;padding:.5rem 1rem}
a{color:#06c}
</style>
</head>
<body>
<h1>Вход</h1>
{{if .Error}}
<div class="msg err">{{.Error}}</div>
{{end}}
<form action="" method="POST">
<input type="hidden" name="page" value="login">
<label for="login">Логин:</label>
<input type="text" id="login" name="login" required>
<label for="password">Пароль:</label>
<input type="password" id="password" name="password" required>
<button type="submit">Войти</button>
</form>
<p><a href="{{.FormPath}}">Назад к форме</a></p>
</body>
</html>`

type formData struct {
	Fio       string
	Phone     string
	Email     string
	Birthdate string
	Gender    string
	Languages []string
	Biography string
	Agreement bool
}

type pageData struct {
	Message      template.HTML
	Form         formData
	Errors       map[string]string
	Auth         bool
	CurrentLogin string
	LoginPath    string
}

type loginPageData struct {
	Error    string
	FormPath string
}

func main() {
	funcMap := template.FuncMap{
		"selected": func(slice []string, val string) bool {
			for _, v := range slice {
				if v == val {
					return true
				}
			}
			return false
		},
	}
	var err error
	formTpl, err = template.New("form").Funcs(funcMap).Parse(formHTML)
	if err != nil {
		log.Fatal("parse form template: ", err)
	}
	loginTpl, err = template.New("login").Parse(loginHTML)
	if err != nil {
		log.Fatal("parse login template: ", err)
	}
	if os.Getenv("GATEWAY_INTERFACE") != "" || os.Getenv("REQUEST_METHOD") != "" {
		runCGI()
		return
	}
	dsn := getDSN()
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("open db: ", err)
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		log.Fatal("ping db: ", err)
	}
	base := getBasePath()
	http.HandleFunc(base+"/", handleForm)
	http.HandleFunc(base+"/login", handleLogin)
	http.HandleFunc(base+"/logout", handleLogout)
	if base != "/" {
		http.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == base {
				http.Redirect(w, r, base+"/", http.StatusFound)
				return
			}
			handleForm(w, r)
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
		writeCGIError("Ошибка БД")
		return
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		writeCGIError("Ошибка БД")
		return
	}
	method := os.Getenv("REQUEST_METHOD")
	if method == "" {
		method = "GET"
	}
	pathInfo := strings.TrimPrefix(os.Getenv("PATH_INFO"), "/")
	if pathInfo == "" && os.Getenv("REQUEST_URI") != "" {
		pathInfo = strings.TrimPrefix(strings.TrimPrefix(os.Getenv("REQUEST_URI"), getBasePath()), "/")
	}
	reqPath := getBasePath()
	if pathInfo != "" {
		reqPath = getBasePath() + "/" + pathInfo
	}
	u := &url.URL{Path: reqPath}
	u.RawQuery = os.Getenv("QUERY_STRING")
	var body io.Reader
	if method == "POST" {
		if n := os.Getenv("CONTENT_LENGTH"); n != "" {
			if size, _ := strconv.Atoi(n); size > 0 && size < 1<<20 {
				body = io.LimitReader(os.Stdin, int64(size))
			}
		}
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		writeCGIError("Ошибка запроса.")
		return
	}
	if method == "POST" && os.Getenv("CONTENT_TYPE") != "" {
		req.Header.Set("Content-Type", os.Getenv("CONTENT_TYPE"))
	}
	req.Header.Set("Cookie", os.Getenv("HTTP_COOKIE"))
	w := &cgiResponseWriter{status: http.StatusOK}
	serveHandler(w, req)
	out := os.Stdout
	statusLine := "Status: " + strconv.Itoa(w.status) + " " + http.StatusText(w.status) + "\r\n"
	if w.status == http.StatusOK {
		statusLine = "Status: 200 OK\r\n"
	}
	out.Write([]byte(statusLine))
	for k, vv := range w.header {
		for _, v := range vv {
			out.Write([]byte(k + ": " + v + "\r\n"))
		}
	}
	out.Write([]byte("\r\n"))
	w.body.WriteTo(out)
}

func writeCGIError(msg string) {
	os.Stdout.Write([]byte("Status: 500 Internal Server Error\r\n"))
	os.Stdout.Write([]byte("Content-Type: text/html; charset=utf-8\r\n\r\n"))
	os.Stdout.Write([]byte("<html><body><h1>" + template.HTMLEscapeString(msg) + "</h1></body></html>"))
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
			dir := path.Dir(s)
			if dir == "/" && s != "" && s != "/" {
				dir = s
			}
			if dir != "." && dir != "" {
				if !strings.HasPrefix(dir, "/") {
					dir = "/" + dir
				}
				return dir
			}
		}
	}
	if b := os.Getenv("BASE_PATH"); b != "" {
		if !strings.HasPrefix(b, "/") {
			b = "/" + b
		}
		return strings.TrimSuffix(b, "/")
	}
	return "/"
}

func serveHandler(w http.ResponseWriter, r *http.Request) {
	base := getBasePath()
	path := strings.TrimPrefix(r.URL.Path, base)
	path = strings.TrimPrefix(path, "/")
	if path == "logout" {
		handleLogout(w, r)
		return
	}
	handleForm(w, r)
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
	s := getEnv("JWT_SECRET", "formapp-secret-change-me")
	return []byte(s)
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

func redirectGET(w http.ResponseWriter, r *http.Request, loc string) {
	if loc == "" {
		base := getBasePath()
		loc = base + "/"
		if base == "/" {
			loc = "/"
		}
		if r.URL.RawQuery != "" {
			loc = loc + "?" + r.URL.RawQuery
		}
	}
	w.Header().Set("Location", loc)
	w.WriteHeader(http.StatusSeeOther)
}

func handleForm(w http.ResponseWriter, r *http.Request) {
	base := getBasePath()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	loginPath := base + "/?login=1"
	if base == "/" {
		loginPath = "/?login=1"
	}
	appID, auth := jwtVerify(getJWT(r))
	var currentLogin string
	if auth {
		currentLogin = getLoginByAppID(appID)
	}

	if r.Method == http.MethodGet {
		if r.URL.Query().Get("login") != "" {
			formPath := base + "/"
			if base == "/" {
				formPath = "/"
			}
			loginTpl.Execute(w, loginPageData{FormPath: formPath})
			return
		}
		if auth {
			form := loadApplication(appID)
			if form != nil {
				renderForm(w, pageData{Form: *form, Auth: true, CurrentLogin: currentLogin, LoginPath: loginPath})
				return
			}
		}
		form, errors, fromError := readCookies(r)
		msg := readSuccessMessageCookie(r)
		if fromError {
			renderForm(w, pageData{Form: form, Errors: errors, Message: msg, LoginPath: loginPath})
			deleteErrorCookies(w)
			return
		}
		if msg != "" {
			renderForm(w, pageData{Form: form, Message: msg, LoginPath: loginPath})
			deleteSuccessMessageCookie(w)
			return
		}
		renderForm(w, pageData{Form: form, LoginPath: loginPath})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderForm(w, pageData{Form: formData{}, Errors: map[string]string{"_": "Ошибка формы."}, LoginPath: loginPath})
		return
	}
	if r.FormValue("page") == "login" {
		login := strings.TrimSpace(r.FormValue("login"))
		password := r.FormValue("password")
		formPath := base + "/"
		if base == "/" {
			formPath = "/"
		}
		if login == "" || password == "" {
			loginTpl.Execute(w, loginPageData{Error: "Введите логин и пароль.", FormPath: formPath})
			return
		}
		appID, hash, salt, ok := findApplicationByLogin(login)
		if !ok || hashPassword(password, salt) != hash {
			loginTpl.Execute(w, loginPageData{Error: "Неверный логин или пароль.", FormPath: formPath})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: jwtCookieName, Value: jwtCreate(appID), Path: base, MaxAge: int(jwtExpireHours * 3600)})
		redirectGET(w, r, formPath)
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
		redirectGET(w, r, "")
		return
	}

	if auth {
		if err := updateApplication(appID, form); err != nil {
			log.Printf("update application: %v", err)
			setSessionErrorCookies(w, form, map[string]string{"_": "Ошибка сохранения."})
			redirectGET(w, r, "")
			return
		}
		setSuccessMessageCookie(w, "Данные обновлены.")
		formPath := base + "/"
		if base == "/" {
			formPath = "/"
		}
		redirectGET(w, r, formPath)
		return
	}

	id, login, password, err := saveApplication(form)
	if err != nil {
		log.Printf("save application: %v", err)
		setSessionErrorCookies(w, form, map[string]string{"_": "Ошибка сохранения."})
		redirectGET(w, r, "")
		return
	}
	_ = id
	setSuccessCookies(w, form)
	setSuccessMessageCookie(w, "Ваш логин: "+login+"<br>Ваш пароль: "+password)
	redirectGET(w, r, "")
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	base := getBasePath()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	formPath := base + "/"
	if base == "/" {
		formPath = "/"
	}
	if r.Method == http.MethodGet {
		loginTpl.Execute(w, loginPageData{FormPath: formPath})
		return
	}
	if err := r.ParseForm(); err != nil {
		loginTpl.Execute(w, loginPageData{Error: "Ошибка формы.", FormPath: formPath})
		return
	}
	login := strings.TrimSpace(r.FormValue("login"))
	password := r.FormValue("password")
	if login == "" || password == "" {
		loginTpl.Execute(w, loginPageData{Error: "Введите логин и пароль.", FormPath: formPath})
		return
	}
	appID, hash, salt, ok := findApplicationByLogin(login)
	if !ok {
		loginTpl.Execute(w, loginPageData{Error: "Неверный логин или пароль.", FormPath: formPath})
		return
	}
	if hashPassword(password, salt) != hash {
		loginTpl.Execute(w, loginPageData{Error: "Неверный логин или пароль.", FormPath: formPath})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: jwtCookieName, Value: jwtCreate(appID), Path: base, MaxAge: int(jwtExpireHours * 3600)})
	redirectGET(w, r, "")
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	base := getBasePath()
	http.SetCookie(w, &http.Cookie{Name: jwtCookieName, Value: "", Path: base, MaxAge: -1})
	redirectGET(w, r, "")
}

func readCookies(r *http.Request) (form formData, errors map[string]string, fromError bool) {
	vals := make(map[string]string)
	for _, c := range r.Cookies() {
		if !strings.HasPrefix(c.Name, cookiePrefix) {
			continue
		}
		key := strings.TrimPrefix(c.Name, cookiePrefix)
		vals[key] = c.Value
	}
	if vals["from"] == "error" {
		fromError = true
		errors = make(map[string]string)
		for k, v := range vals {
			if strings.HasPrefix(k, "err_") && v != "" {
				errors[strings.TrimPrefix(k, "err_")] = cookieDecode(v)
			}
		}
	}
	form.Fio = cookieDecode(vals["fio"])
	form.Phone = cookieDecode(vals["phone"])
	form.Email = cookieDecode(vals["email"])
	form.Birthdate = cookieDecode(vals["birthdate"])
	form.Gender = cookieDecode(vals["gender"])
	form.Biography = cookieDecode(vals["bio"])
	form.Agreement = cookieDecode(vals["agreement"]) == "on"
	if s := cookieDecode(vals["languages"]); s != "" {
		form.Languages = strings.Split(s, ",")
	}
	return form, errors, fromError
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

func setSuccessCookies(w http.ResponseWriter, form formData) {
	setFormCookies(w, form, cookieMaxAgeYear)
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

func deleteErrorCookies(w http.ResponseWriter) {
	cookiePath := getBasePath()
	for _, n := range []string{"from", "fio", "phone", "email", "birthdate", "gender", "languages", "bio", "agreement", "err_fio", "err_phone", "err_email", "err_birthdate", "err_gender", "err_languages", "err_bio", "err_agreement"} {
		http.SetCookie(w, &http.Cookie{Name: cookiePrefix + n, Value: "", Path: cookiePath, MaxAge: -1})
	}
}

func setSuccessMessageCookie(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "msg", Value: cookieEncode(msg), Path: getBasePath()})
}

func readSuccessMessageCookie(r *http.Request) template.HTML {
	for _, c := range r.Cookies() {
		if c.Name == cookiePrefix+"msg" && c.Value != "" {
			return template.HTML(cookieDecode(c.Value))
		}
	}
	return ""
}

func deleteSuccessMessageCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "msg", Value: "", Path: getBasePath(), MaxAge: -1})
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
	err := db.QueryRow("SELECT fio, phone, email, birthdate, gender, biography, contract_agreed FROM application WHERE id=?", appID).Scan(&f.Fio, &f.Phone, &f.Email, &f.Birthdate, &f.Gender, &bio, &agreed)
	if err != nil {
		return nil
	}
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

func getLoginByAppID(appID int64) string {
	var login string
	if err := db.QueryRow("SELECT login FROM application WHERE id=?", appID).Scan(&login); err != nil {
		return ""
	}
	return login
}

func renderForm(w http.ResponseWriter, data pageData) {
	if data.Errors != nil && data.Errors["_"] != "" {
		data.Message = template.HTML(template.HTMLEscapeString(data.Errors["_"]))
	}
	if data.LoginPath == "" {
		data.LoginPath = getBasePath() + "/?login=1"
	}
	if err := formTpl.Execute(w, data); err != nil {
		log.Printf("template execute: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
