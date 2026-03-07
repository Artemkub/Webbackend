package main

import (
	"bytes"
	"database/sql"
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
	maxFioLen      = 150
	maxBioLen      = 5000
	maxPhoneLen    = 30
	maxEmailLen    = 255
	minLangID      = 1
	maxLangID      = 12
	allowedGenders = "male female"
)

var (
	db         *sql.DB
	formTpl    *template.Template
	fioRegex   = regexp.MustCompile(`^[\p{L}\s\-]+$`)
	phoneRegex = regexp.MustCompile(`^[\d\s+\-()]+$`)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
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
.msg.err{background:#fee;border:1px solid #c00}
.msg.ok{background:#efe;border:1px solid #0a0}
label{display:block;margin-top:.75rem;margin-bottom:.25rem}
input[type="text"],input[type="tel"],input[type="email"],input[type="date"],select,textarea{width:100%;padding:.4rem}
select[multiple]{min-height:120px}
textarea{min-height:80px;resize:vertical}
button{margin-top:1rem;padding:.5rem 1rem}
</style>
</head>
<body>
<h1>Анкета</h1>
{{if .Message}}
<div class="msg {{if .MessageError}}err{{else}}ok{{end}}">{{.Message}}</div>
{{end}}
<form action="" method="POST">
<label for="fio">ФИО:</label>
<input type="text" id="fio" name="fio" maxlength="150" value="{{.Form.Fio}}">
<label for="phone">Телефон:</label>
<input type="tel" id="phone" name="phone" value="{{.Form.Phone}}">
<label for="email">E-mail:</label>
<input type="email" id="email" name="email" value="{{.Form.Email}}">
<label for="birthdate">Дата рождения:</label>
<input type="date" id="birthdate" name="birthdate" value="{{.Form.Birthdate}}">
<label>Пол:</label>
<input type="radio" id="male" name="gender" value="male" {{if eq .Form.Gender "male"}}checked{{end}}>
<label for="male">Мужской</label>
<input type="radio" id="female" name="gender" value="female" {{if eq .Form.Gender "female"}}checked{{end}}>
<label for="female">Женский</label>
<label for="languages">Любимый язык программирования:</label>
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
<label for="bio">Биография:</label>
<textarea id="bio" name="bio" rows="5" cols="40">{{.Form.Biography}}</textarea>
<input type="checkbox" id="agreement" name="agreement" value="on" {{if .Form.Agreement}}checked{{end}}>
<label for="agreement">С контрактом ознакомлен(а)</label>
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
	Message      template.HTML
	MessageError bool
	Form         formData
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

func (c *cgiResponseWriter) WriteHeader(code int) {
	c.status = code
}

func (c *cgiResponseWriter) Write(p []byte) (int, error) {
	return c.body.Write(p)
}

func runCGI() {
	var err error
	db, err = sql.Open("mysql", getDSN())
	if err != nil {
		writeCGIError("Ошибка подключения к БД")
		return
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		writeCGIError("Ошибка подключения к БД")
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
		writeCGIError("Ошибка запроса")
		return
	}
	if method == "POST" && os.Getenv("CONTENT_TYPE") != "" {
		req.Header.Set("Content-Type", os.Getenv("CONTENT_TYPE"))
	}
	w := &cgiResponseWriter{status: http.StatusOK}
	handleForm(w, req)
	out := os.Stdout
	statusLine := "Status: 200 OK\r\n"
	if w.status != http.StatusOK {
		statusLine = "Status: " + strconv.Itoa(w.status) + " " + http.StatusText(w.status) + "\r\n"
	}
	out.Write([]byte(statusLine))
	if w.header != nil {
		if ct := w.header.Get("Content-Type"); ct != "" {
			out.Write([]byte("Content-Type: " + ct + "\r\n"))
		} else {
			out.Write([]byte("Content-Type: text/html; charset=utf-8\r\n"))
		}
	} else {
		out.Write([]byte("Content-Type: text/html; charset=utf-8\r\n"))
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

func handleForm(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodGet {
		renderForm(w, pageData{Form: formData{}})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderForm(w, pageData{
			Message:      template.HTML(template.HTMLEscapeString("Ошибка обработки формы.")),
			MessageError: true,
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

	errs := validate(form)
	if len(errs) > 0 {
		var b strings.Builder
		b.WriteString("Исправьте ошибки:<ul>")
		for _, e := range errs {
			b.WriteString("<li>")
			b.WriteString(template.HTMLEscapeString(e))
			b.WriteString("</li>")
		}
		b.WriteString("</ul>")
		renderForm(w, pageData{
			Message:      template.HTML(b.String()),
			MessageError: true,
			Form:         form,
		})
		return
	}
	if _, err := saveApplication(form); err != nil {
		log.Printf("save application: %v", err)
		renderForm(w, pageData{
			Message:      template.HTML(template.HTMLEscapeString("Ошибка сохранения в базу данных.")),
			MessageError: true,
			Form:         form,
		})
		return
	}
	renderForm(w, pageData{
		Message:      template.HTML(template.HTMLEscapeString("Данные успешно сохранены.")),
		MessageError: false,
		Form:         formData{},
	})
}

func validate(f formData) (errs []string) {
	if f.Fio == "" {
		errs = append(errs, "ФИО: поле обязательно для заполнения.")
	} else {
		if len(f.Fio) > maxFioLen {
			errs = append(errs, "ФИО: не более "+strconv.Itoa(maxFioLen)+" символов.")
		}
		if !fioRegex.MatchString(f.Fio) {
			errs = append(errs, "ФИО: допускаются только буквы, пробелы и дефис.")
		}
	}
	if f.Phone == "" {
		errs = append(errs, "Телефон: поле обязательно для заполнения.")
	} else {
		if len(f.Phone) > maxPhoneLen {
			errs = append(errs, "Телефон: не более "+strconv.Itoa(maxPhoneLen)+" символов.")
		}
		if !phoneRegex.MatchString(f.Phone) {
			errs = append(errs, "Телефон: допускаются только цифры и символы + - ( ).")
		}
	}
	if f.Email == "" {
		errs = append(errs, "E-mail: поле обязательно для заполнения.")
	} else {
		if len(f.Email) > maxEmailLen {
			errs = append(errs, "E-mail: не более "+strconv.Itoa(maxEmailLen)+" символов.")
		}
		if !emailRegex.MatchString(f.Email) {
			errs = append(errs, "E-mail: неверный формат.")
		}
	}
	if f.Birthdate == "" {
		errs = append(errs, "Дата рождения: поле обязательно для заполнения.")
	} else if _, err := parseDate(f.Birthdate); err != nil {
		errs = append(errs, "Дата рождения: неверный формат даты.")
	}
	if f.Gender == "" {
		errs = append(errs, "Пол: выберите значение.")
	} else if !strings.Contains(allowedGenders, f.Gender) {
		errs = append(errs, "Пол: недопустимое значение.")
	}
	if len(f.Languages) == 0 {
		errs = append(errs, "Любимый язык программирования: выберите хотя бы один язык.")
	} else {
		for _, s := range f.Languages {
			id, err := strconv.Atoi(s)
			if err != nil || id < minLangID || id > maxLangID {
				errs = append(errs, "Любимый язык программирования: выбран недопустимый язык.")
				break
			}
		}
	}
	if len(f.Biography) > maxBioLen {
		errs = append(errs, "Биография: не более "+strconv.Itoa(maxBioLen)+" символов.")
	}
	if !f.Agreement {
		errs = append(errs, "Необходимо подтвердить ознакомление с контрактом.")
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
	if y < 1900 || y > 2026 || m < 1 || m > 12 || d < 1 || d > 31 {
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
	if err := formTpl.Execute(w, data); err != nil {
		log.Printf("template execute: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
