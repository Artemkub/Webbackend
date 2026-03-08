package main

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const (
	cookiePrefix   = "fa_"
	maxFioLen      = 150
	maxBioLen      = 5000
	maxPhoneLen    = 30
	maxEmailLen    = 255
	minLangID      = 1
	maxLangID      = 12
	cookieMaxAgeYear = 365 * 24 * 3600
)

var (
	db         *sql.DB
	formTpl    *template.Template
	fioRegex   = regexp.MustCompile(`^[\p{L}\s\-]+$`)
	phoneRegex = regexp.MustCompile(`^[\d\s+\-()+]+$`)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	dateRegex  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
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
.field-error input, .field-error select, .field-error textarea{border-color:#c00;background:#fff8f8}
.err-msg{display:block;color:#c00;font-size:0.9em;margin-top:0.2rem}
label{display:block;margin-top:.75rem;margin-bottom:.25rem}
input[type="text"],input[type="tel"],input[type="email"],input[type="date"],select,textarea{width:100%;padding:.4rem;border:1px solid #ccc}
select[multiple]{min-height:120px}
textarea{min-height:80px;resize:vertical}
button{margin-top:1rem;padding:.5rem 1rem}
</style>
</head>
<body>
<h1>Анкета</h1>
{{if .Message}}
<div class="msg ok">{{.Message}}</div>
{{end}}
<form action="" method="POST">
<div class="{{if .Errors.fio}}field-error{{end}}">
<label for="fio">ФИО:</label>
{{if .Errors.fio}}<span class="err-msg">{{.Errors.fio }}</span>{{end}}
<input type="text" id="fio" name="fio" maxlength="150" value="{{.Form.Fio}}">
</div>
<div class="{{if .Errors.phone}}field-error{{end}}">
<label for="phone">Телефон:</label>
{{if .Errors.phone}}<span class="err-msg">{{.Errors.phone }}</span>{{end}}
<input type="tel" id="phone" name="phone" value="{{.Form.Phone}}">
</div>
<div class="{{if .Errors.email}}field-error{{end}}">
<label for="email">E-mail:</label>
{{if .Errors.email}}<span class="err-msg">{{.Errors.email }}</span>{{end}}
<input type="email" id="email" name="email" value="{{.Form.Email}}">
</div>
<div class="{{if .Errors.birthdate}}field-error{{end}}">
<label for="birthdate">Дата рождения:</label>
{{if .Errors.birthdate}}<span class="err-msg">{{.Errors.birthdate }}</span>{{end}}
<input type="date" id="birthdate" name="birthdate" value="{{.Form.Birthdate}}">
</div>
<div class="{{if .Errors.gender}}field-error{{end}}">
<label>Пол:</label>
{{if .Errors.gender}}<span class="err-msg">{{.Errors.gender }}</span>{{end}}
<input type="radio" id="male" name="gender" value="male" {{if eq .Form.Gender "male"}}checked{{end}}>
<label for="male">Мужской</label>
<input type="radio" id="female" name="gender" value="female" {{if eq .Form.Gender "female"}}checked{{end}}>
<label for="female">Женский</label>
</div>
<div class="{{if .Errors.languages}}field-error{{end}}">
<label for="languages">Любимый язык программирования:</label>
{{if .Errors.languages}}<span class="err-msg">{{.Errors.languages }}</span>{{end}}
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
{{if .Errors.bio}}<span class="err-msg">{{.Errors.bio }}</span>{{end}}
<textarea id="bio" name="bio" rows="5" cols="40">{{.Form.Biography}}</textarea>
</div>
<div class="{{if .Errors.agreement}}field-error{{end}}">
{{if .Errors.agreement}}<span class="err-msg">{{.Errors.agreement }}</span>{{end}}
<input type="checkbox" id="agreement" name="agreement" value="on" {{if .Form.Agreement}}checked{{end}}>
<label for="agreement">С контрактом ознакомлен(а)</label>
</div>
<br>
<button type="submit">Сохранить</button>
</form>
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
	Message template.HTML
	Form    formData
	Errors  map[string]string
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
		log.Fatal("parse template: ", err)
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
	http.HandleFunc("/", handleForm)
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
	u := &url.URL{Path: "/"}
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
	handleForm(w, req)
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

func handleForm(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if r.Method == http.MethodGet {
		form, errors, fromError := readCookies(r)
		msg := readSuccessMessageCookie(r)
		if fromError {
			renderForm(w, pageData{Form: form, Errors: errors, Message: msg})
			deleteErrorCookies(w)
			return
		}
		if msg != "" {
			renderForm(w, pageData{Form: form, Errors: nil, Message: msg})
			deleteSuccessMessageCookie(w)
			return
		}
		renderForm(w, pageData{Form: form, Errors: nil})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderForm(w, pageData{
			Form:   formData{},
			Errors: map[string]string{"_": "Ошибка формы."},
		})
		return
	}
	form := formData{
		Fio:       strings.TrimSpace(r.FormValue("fio")),
		Phone:     strings.TrimSpace(r.FormValue("phone")),
		Email:     strings.TrimSpace(r.FormValue("email")),
		Birthdate: strings.TrimSpace(r.FormValue("birthdate")),
		Gender:    strings.TrimSpace(r.FormValue("gender")),
		Biography: strings.TrimSpace(r.FormValue("bio")),
		Agreement: r.FormValue("agreement") == "on",
	}
	form.Languages = r.Form["languages"]

	errors := validateRegex(form)
	if len(errors) > 0 {
		setSessionErrorCookies(w, form, errors)
		redirectGET(w, r)
		return
	}
	if _, err := saveApplication(form); err != nil {
		log.Printf("save application: %v", err)
		errors := map[string]string{"_": "Ошибка сохранения."}
		setSessionErrorCookies(w, form, errors)
		redirectGET(w, r)
		return
	}
	setSuccessCookies(w, form)
	setSuccessMessageCookie(w)
	redirectGET(w, r)
}

func redirectGET(w http.ResponseWriter, r *http.Request) {
	loc := "/"
	if r.URL.RawQuery != "" {
		loc = "/?" + r.URL.RawQuery
	}
	w.Header().Set("Location", loc)
	w.WriteHeader(http.StatusSeeOther)
}

func readCookies(r *http.Request) (form formData, errors map[string]string, fromError bool) {
	cookies := r.Cookies()
	vals := make(map[string]string)
	for _, c := range cookies {
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
	path := "/"
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "from", Value: "error", Path: path})
	setFormCookies(w, form, 0)
	for k, v := range errors {
		if k == "_" {
			continue
		}
		http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "err_" + k, Value: cookieEncode(v), Path: path})
	}
}

func setSuccessCookies(w http.ResponseWriter, form formData) {
	setFormCookies(w, form, cookieMaxAgeYear)
}

func setFormCookies(w http.ResponseWriter, form formData, maxAge int) {
	path := "/"
	opts := []struct{ name, val string }{
		{"fio", form.Fio},
		{"phone", form.Phone},
		{"email", form.Email},
		{"birthdate", form.Birthdate},
		{"gender", form.Gender},
		{"languages", strings.Join(form.Languages, ",")},
		{"bio", form.Biography},
	}
	if form.Agreement {
		opts = append(opts, struct{ name, val string }{"agreement", "on"})
	} else {
		opts = append(opts, struct{ name, val string }{"agreement", ""})
	}
	for _, o := range opts {
		c := &http.Cookie{Name: cookiePrefix + o.name, Value: cookieEncode(o.val), Path: path}
		if maxAge > 0 {
			c.MaxAge = maxAge
		}
		http.SetCookie(w, c)
	}
}

func deleteErrorCookies(w http.ResponseWriter) {
	path := "/"
	names := []string{"from", "fio", "phone", "email", "birthdate", "gender", "languages", "bio", "agreement",
		"err_fio", "err_phone", "err_email", "err_birthdate", "err_gender", "err_languages", "err_bio", "err_agreement"}
	for _, n := range names {
		http.SetCookie(w, &http.Cookie{Name: cookiePrefix + n, Value: "", Path: path, MaxAge: -1})
	}
}

func setSuccessMessageCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "msg", Value: "ok", Path: "/"})
}

func readSuccessMessageCookie(r *http.Request) template.HTML {
	for _, c := range r.Cookies() {
		if c.Name == cookiePrefix+"msg" && c.Value == "ok" {
			return template.HTML(template.HTMLEscapeString("Данные сохранены."))
		}
	}
	return ""
}

func deleteSuccessMessageCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookiePrefix + "msg", Value: "", Path: "/", MaxAge: -1})
}

func validateRegex(f formData) map[string]string {
	errs := make(map[string]string)

	if f.Fio == "" {
		errs["fio"] = "Поле обязательно."
	} else {
		if len(f.Fio) > maxFioLen {
			errs["fio"] = "Не более 150 символов."
		} else if !fioRegex.MatchString(f.Fio) {
			errs["fio"] = "Только буквы, пробелы и дефис."
		}
	}

	if f.Phone == "" {
		errs["phone"] = "Поле обязательно."
	} else {
		if len(f.Phone) > maxPhoneLen {
			errs["phone"] = "Не более 30 символов."
		} else if !phoneRegex.MatchString(f.Phone) {
			errs["phone"] = "Только цифры и + - ( )."
		}
	}

	if f.Email == "" {
		errs["email"] = "Поле обязательно."
	} else {
		if len(f.Email) > maxEmailLen {
			errs["email"] = "Не более 255 символов."
		} else if !emailRegex.MatchString(f.Email) {
			errs["email"] = "Недопустимый формат e-mail."
		}
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

func saveApplication(f formData) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare("INSERT INTO application (fio, phone, email, birthdate, gender, biography, contract_agreed) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	agreed := 0
	if f.Agreement {
		agreed = 1
	}
	res, err := stmt.Exec(f.Fio, f.Phone, f.Email, f.Birthdate, f.Gender, f.Biography, agreed)
	if err != nil {
		return 0, err
	}
	appID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	linkStmt, err := tx.Prepare("INSERT INTO application_language (application_id, language_id) VALUES (?, ?)")
	if err != nil {
		return 0, err
	}
	defer linkStmt.Close()
	for _, langIDStr := range f.Languages {
		if _, err = linkStmt.Exec(appID, langIDStr); err != nil {
			return 0, err
		}
	}
	return appID, tx.Commit()
}

func renderForm(w http.ResponseWriter, data pageData) {
	if data.Errors != nil && data.Errors["_"] != "" {
		data.Message = template.HTML(template.HTMLEscapeString(data.Errors["_"]))
	}
	if err := formTpl.Execute(w, data); err != nil {
		log.Printf("template execute: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
