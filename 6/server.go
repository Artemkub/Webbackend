package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

	_ "github.com/go-sql-driver/mysql"
)

const (
	maxFioLen   = 150
	maxBioLen   = 5000
	maxPhoneLen = 30
	maxEmailLen = 255
	minLangID   = 1
	maxLangID   = 12
)

var (
	db          *sql.DB
	listTpl     *template.Template
	editTpl     *template.Template
	fioRegex    = regexp.MustCompile(`^[\p{L}\s\-]+$`)
	phoneRegex  = regexp.MustCompile(`^[\d\s+\-()+]+$`)
	emailRegex  = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	dateRegex   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	genderRegex = regexp.MustCompile(`^(male|female)$`)
	langIDRegex = regexp.MustCompile(`^(1[0-2]|[1-9])$`)
)

type appRecord struct {
	ID            int64
	Login         string
	Fio           string
	Phone         string
	Email         string
	Birthdate     string
	Gender        string
	Biography     string
	Contract      bool
	LanguageNames string
}

type listData struct {
	Apps  []appRecord
	Stats []langStat
}

type langStat struct {
	LangName string
	Count    int
}

type formData struct {
	Fio       string
	Phone     string
	Email     string
	Birthdate string
	Gender    string
	Biography string
	Languages []string
	Agreement bool
}

type editData struct {
	App    appRecord
	Form   formData
	Errors map[string]string
}

const listHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Панель администратора</title>
<style>
body{font-family:sans-serif;max-width:960px;margin:2rem auto;padding:0 1rem}
h1,h2{margin-top:0}
table{border-collapse:collapse;width:100%;margin:1rem 0}
th,td{border:1px solid #ccc;padding:.4rem .6rem;text-align:left}
th{background:#eee}
.stats{margin:2rem 0}
.stats table{width:auto}
a{color:#06c;margin-right:1rem}
.msg{background:#efe;border:1px solid #0a0;padding:.5rem;margin-bottom:1rem;border-radius:6px}
</style>
</head>
<body>
<h1>Панель администратора</h1>
{{if .Message}}<div class="msg">{{.Message}}</div>{{end}}
<h2>Статистика по языкам программирования</h2>
<div class="stats">
<table>
<thead><tr><th>Язык</th><th>Количество пользователей</th></tr></thead>
<tbody>
{{range .Stats}}<tr><td>{{.LangName}}</td><td>{{.Count}}</td></tr>{{end}}
</tbody>
</table>
</div>
<h2>Все заявки</h2>
<table>
<thead>
<tr><th>ID</th><th>Логин</th><th>ФИО</th><th>Телефон</th><th>E-mail</th><th>Дата рожд.</th><th>Пол</th><th>Языки</th><th>Действия</th></tr>
</thead>
<tbody>
{{range .Apps}}
<tr>
<td>{{.ID}}</td><td>{{.Login}}</td><td>{{.Fio}}</td><td>{{.Phone}}</td><td>{{.Email}}</td><td>{{.Birthdate}}</td><td>{{.Gender}}</td><td>{{.LanguageNames}}</td>
<td><a href="{{$.EditBase}}?id={{.ID}}">Редактировать</a>
<form action="{{$.DeleteBase}}" method="POST" style="display:inline" onsubmit="return confirm('Удалить заявку?')"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Удалить</button></form></td>
</tr>
{{end}}
</tbody>
</table>
</body>
</html>`

const editHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>Редактирование заявки</title>
<style>
body{font-family:sans-serif;max-width:480px;margin:2rem auto;padding:0 1rem}
h1{margin-top:0}
label{display:block;margin-top:.75rem;margin-bottom:.25rem}
input[type="text"],input[type="tel"],input[type="email"],input[type="date"],select,textarea{width:100%;padding:.4rem;border:1px solid #ccc}
select[multiple]{min-height:120px}
textarea{min-height:80px;resize:vertical}
.field-error input,.field-error select,.field-error textarea{border-color:#c00;background:#fff8f8}
.err-msg{color:#c00;font-size:0.9em}
button{margin-top:1rem;padding:.5rem 1rem}
a{color:#06c}
</style>
</head>
<body>
<h1>Редактирование заявки #{{.App.ID}}</h1>
<p><a href="{{.BackURL}}">← К списку</a></p>
{{if .Errors._}}<p class="err-msg">{{.Errors._}}</p>{{end}}
<form action="" method="POST">
<input type="hidden" name="id" value="{{.App.ID}}">
<div class="{{if .Errors.fio}}field-error{{end}}"><label>ФИО:</label>{{if .Errors.fio}}<span class="err-msg">{{.Errors.fio}}</span>{{end}}<input type="text" name="fio" maxlength="150" value="{{.Form.Fio}}"></div>
<div class="{{if .Errors.phone}}field-error{{end}}"><label>Телефон:</label>{{if .Errors.phone}}<span class="err-msg">{{.Errors.phone}}</span>{{end}}<input type="tel" name="phone" value="{{.Form.Phone}}"></div>
<div class="{{if .Errors.email}}field-error{{end}}"><label>E-mail:</label>{{if .Errors.email}}<span class="err-msg">{{.Errors.email}}</span>{{end}}<input type="email" name="email" value="{{.Form.Email}}"></div>
<div class="{{if .Errors.birthdate}}field-error{{end}}"><label>Дата рождения:</label>{{if .Errors.birthdate}}<span class="err-msg">{{.Errors.birthdate}}</span>{{end}}<input type="date" name="birthdate" value="{{.Form.Birthdate}}"></div>
<div class="{{if .Errors.gender}}field-error{{end}}"><label>Пол:</label>{{if .Errors.gender}}<span class="err-msg">{{.Errors.gender}}</span>{{end}}
<input type="radio" id="male" name="gender" value="male" {{if eq .Form.Gender "male"}}checked{{end}}><label for="male" style="display:inline">Мужской</label>
<input type="radio" id="female" name="gender" value="female" {{if eq .Form.Gender "female"}}checked{{end}}><label for="female" style="display:inline">Женский</label>
</div>
<div class="{{if .Errors.languages}}field-error{{end}}"><label>Любимые языки:</label>{{if .Errors.languages}}<span class="err-msg">{{.Errors.languages}}</span>{{end}}
<select name="languages" multiple size="6">
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
<div class="{{if .Errors.bio}}field-error{{end}}"><label>Биография:</label>{{if .Errors.bio}}<span class="err-msg">{{.Errors.bio}}</span>{{end}}<textarea name="bio" rows="5">{{.Form.Biography}}</textarea></div>
<div class="{{if .Errors.agreement}}field-error{{end}}"><input type="checkbox" id="agreement" name="agreement" value="on" {{if .Form.Agreement}}checked{{end}}><label for="agreement" style="display:inline">С контрактом ознакомлен(а)</label></div>
<button type="submit">Сохранить</button>
</form>
</body>
</html>`

func main() {
	funcMap := template.FuncMap{"selected": func(sl []string, v string) bool {
		for _, s := range sl {
			if s == v {
				return true
			}
		}
		return false
	}}
	var err error
	t := template.New("list").Funcs(funcMap)
	listTpl, err = t.Parse(listHTML)
	if err != nil {
		log.Fatal(err)
	}
	t = template.New("edit").Funcs(funcMap)
	editTpl, err = t.Parse(editHTML)
	if err != nil {
		log.Fatal(err)
	}

	if os.Getenv("GATEWAY_INTERFACE") != "" || (os.Getenv("REQUEST_METHOD") != "" && os.Getenv("SCRIPT_NAME") != "") {
		runCGI()
		return
	}
	db, err = sql.Open("mysql", getDSN())
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/6/", withBasicAuth(serveAdmin))
	http.HandleFunc("/6", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/6/", http.StatusSeeOther)
	})
	log.Println("Admin server on :8086 (path /6/)")
	log.Fatal(http.ListenAndServe(":8086", nil))
}

type cgiResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (c *cgiResponseWriter) Header() http.Header {
	if c.header == nil {
		c.header = make(http.Header)
	}
	return c.header
}
func (c *cgiResponseWriter) WriteHeader(code int)        { c.status = code }
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
	base := getBasePath()
	pathInfo := strings.TrimPrefix(os.Getenv("PATH_INFO"), "/")
	if pathInfo == "" && os.Getenv("REQUEST_URI") != "" {
		pathInfo = strings.TrimPrefix(strings.TrimPrefix(os.Getenv("REQUEST_URI"), base), "/")
		pathInfo = strings.TrimPrefix(pathInfo, "/")
	}
	reqPath := base
	if pathInfo != "" {
		reqPath = base + "/" + pathInfo
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
	if auth := os.Getenv("HTTP_AUTHORIZATION"); auth != "" {
		req.Header.Set("Authorization", auth)
	} else if auth := os.Getenv("REDIRECT_HTTP_AUTHORIZATION"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := &cgiResponseWriter{status: http.StatusOK}
	serveAdmin(w, req)
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
	return "/6"
}

func hashPassword(password, salt string) string {
	h := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(h[:])
}

func verifyAdmin(login, password string) bool {
	var hash, salt string
	err := db.QueryRow("SELECT password_hash, salt FROM admin WHERE login=?", login).Scan(&hash, &salt)
	if err != nil {
		return false
	}
	return hashPassword(password, salt) == hash
}

func basicAuthFailed(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("WWW-Authenticate", `Basic realm="Admin"`)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("<html><body><h1>Требуется авторизация</h1></body></html>"))
}

func withBasicAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || !verifyAdmin(user, pass) {
			basicAuthFailed(w)
			return
		}
		_ = user
		h(w, r)
	}
}

func serveAdmin(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()
	if !ok || !verifyAdmin(user, pass) {
		basicAuthFailed(w)
		return
	}
	_ = user
	base := getBasePath()
	if r.URL.Path != base && r.URL.Path != base+"/" {
		pathSuffix := strings.TrimPrefix(r.URL.Path, base)
		pathSuffix = strings.TrimPrefix(pathSuffix, "/")
		parts := strings.SplitN(pathSuffix, "/", 2)
		action := parts[0]
		if action == "edit" {
			handleEdit(w, r, base)
			return
		}
		if action == "delete" {
			handleDelete(w, r, base)
			return
		}
	}
	// List (index) — only GET
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	handleList(w, r, base)
}

func handleList(w http.ResponseWriter, r *http.Request, base string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	apps, err := listAllApplications()
	if err != nil {
		log.Printf("list applications: %v", err)
		http.Error(w, "Ошибка загрузки", http.StatusInternalServerError)
		return
	}
	stats, err := getLanguageStats()
	if err != nil {
		log.Printf("language stats: %v", err)
		stats = nil
	}
	msg := r.URL.Query().Get("msg")
	if msg == "deleted" {
		msg = "Заявка удалена."
	} else if msg == "saved" {
		msg = "Данные сохранены."
	}
	data := struct {
		Apps       []appRecord
		Stats      []langStat
		Message    string
		EditBase   string
		DeleteBase string
	}{Apps: apps, Stats: stats, Message: msg, EditBase: base + "/edit", DeleteBase: base + "/delete"}
	if err := listTpl.Execute(w, data); err != nil {
		log.Printf("list template: %v", err)
	}
}

func handleEdit(w http.ResponseWriter, r *http.Request, base string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	idStr := r.URL.Query().Get("id")
	if r.Method == http.MethodPost {
		idStr = r.PostFormValue("id")
	}
	if idStr == "" {
		http.Redirect(w, r, base+"/", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, base+"/", http.StatusSeeOther)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			renderEditForm(w, id, formData{}, map[string]string{"_": "Ошибка формы."}, base)
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
		errors := validateForm(form)
		if len(errors) > 0 {
			app := getAppByID(id)
			if app != nil {
				renderEditFormWithApp(w, id, *app, form, errors, base)
			} else {
				redirectTo(w, r, base+"/")
			}
			return
		}
		if err := updateApplication(id, form); err != nil {
			log.Printf("update application: %v", err)
			renderEditForm(w, id, form, map[string]string{"_": "Ошибка сохранения."}, base)
			return
		}
		redirectTo(w, r, base+"/?msg=saved")
		return
	}

	app := getAppByID(id)
	if app == nil {
		http.Redirect(w, r, base+"/", http.StatusSeeOther)
		return
	}
	form := loadFormByAppID(id)
	renderEditFormWithApp(w, id, *app, form, nil, base)
}

func redirectTo(w http.ResponseWriter, r *http.Request, loc string) {
	if w.Header().Get("Location") == "" {
		w.Header().Set("Location", loc)
		w.WriteHeader(http.StatusSeeOther)
	}
}

func handleDelete(w http.ResponseWriter, r *http.Request, base string) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
	}
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		idStr = r.PostFormValue("id")
	}
	if idStr == "" {
		redirectTo(w, r, base+"/")
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		redirectTo(w, r, base+"/")
		return
	}
	if err := deleteApplication(id); err != nil {
		log.Printf("delete application: %v", err)
	}
	redirectTo(w, r, base+"/?msg=deleted")
}

func listAllApplications() ([]appRecord, error) {
	rows, err := db.Query("SELECT id, login, fio, phone, email, birthdate, gender, biography, contract_agreed FROM application ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []appRecord
	for rows.Next() {
		var a appRecord
		var bio sql.NullString
		var agreed int
		if err := rows.Scan(&a.ID, &a.Login, &a.Fio, &a.Phone, &a.Email, &a.Birthdate, &a.Gender, &bio, &agreed); err != nil {
			return nil, err
		}
		if bio.Valid {
			a.Biography = bio.String
		}
		a.Contract = agreed != 0
		names, _ := getLanguageNamesForApp(a.ID)
		a.LanguageNames = strings.Join(names, ", ")
		list = append(list, a)
	}
	return list, rows.Err()
}

func getLanguageNamesForApp(appID int64) ([]string, error) {
	rows, err := db.Query("SELECT pl.name FROM application_language al JOIN programming_language pl ON pl.id = al.language_id WHERE al.application_id = ? ORDER BY pl.name", appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func getLanguageStats() ([]langStat, error) {
	rows, err := db.Query("SELECT pl.name, COUNT(al.application_id) AS cnt FROM programming_language pl LEFT JOIN application_language al ON al.language_id = pl.id GROUP BY pl.id ORDER BY pl.name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []langStat
	for rows.Next() {
		var s langStat
		if err := rows.Scan(&s.LangName, &s.Count); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func getAppByID(id int64) *appRecord {
	var a appRecord
	var bio sql.NullString
	var agreed int
	err := db.QueryRow("SELECT id, login, fio, phone, email, birthdate, gender, biography, contract_agreed FROM application WHERE id=?", id).Scan(&a.ID, &a.Login, &a.Fio, &a.Phone, &a.Email, &a.Birthdate, &a.Gender, &bio, &agreed)
	if err != nil {
		return nil
	}
	if bio.Valid {
		a.Biography = bio.String
	}
	a.Contract = agreed != 0
	return &a
}

func loadFormByAppID(appID int64) formData {
	var f formData
	var bio sql.NullString
	var agreed int
	err := db.QueryRow("SELECT fio, phone, email, birthdate, gender, biography, contract_agreed FROM application WHERE id=?", appID).Scan(&f.Fio, &f.Phone, &f.Email, &f.Birthdate, &f.Gender, &bio, &agreed)
	if err != nil {
		return formData{}
	}
	if bio.Valid {
		f.Biography = bio.String
	}
	f.Agreement = agreed != 0
	rows, err := db.Query("SELECT language_id FROM application_language WHERE application_id=?", appID)
	if err != nil {
		return f
	}
	defer rows.Close()
	for rows.Next() {
		var lid int
		rows.Scan(&lid)
		f.Languages = append(f.Languages, strconv.Itoa(lid))
	}
	return f
}

func renderEditForm(w http.ResponseWriter, id int64, form formData, errors map[string]string, base string) {
	app := getAppByID(id)
	if app == nil {
		return
	}
	renderEditFormWithApp(w, id, *app, form, errors, base)
}

func renderEditFormWithApp(w http.ResponseWriter, id int64, app appRecord, form formData, errors map[string]string, base string) {
	if errors == nil {
		errors = make(map[string]string)
	}
	data := editData{App: app, Form: form, Errors: errors}
	type editDataWithBack struct {
		editData
		BackURL string
	}
	wrap := editDataWithBack{editData: data, BackURL: base + "/"}
	if err := editTpl.Execute(w, wrap); err != nil {
		log.Printf("edit template: %v", err)
	}
}

func validateForm(f formData) map[string]string {
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

func deleteApplication(appID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec("DELETE FROM application_language WHERE application_id=?", appID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM application WHERE id=?", appID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
