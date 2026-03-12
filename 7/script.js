"use strict";

function getBasePath() {
    var p = window.location.pathname;
    p = p.replace(/\/index\.html$/i, "").replace(/\/$/, "");
    return p || "/7";
}

var BASE = (typeof window.API_BASE_PATH !== "undefined" && window.API_BASE_PATH) ? window.API_BASE_PATH : getBasePath();
var STORAGE_KEY = "contactFormData";
var FIO_REGEX = /^[\p{L}\s\-]+$/u;
var PHONE_REGEX = /^[\d\s+\-()+]+$/;
var EMAIL_REGEX = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
var DATE_REGEX = /^\d{4}-\d{2}-\d{2}$/;
var LANG_IDS = ["1","2","3","4","5","6","7","8","9","10","11","12"];

var mobileMenuToggle = document.getElementById("mobileMenuToggle");
var mobileMenu = document.getElementById("mobileMenu");
var contactForm = document.getElementById("contactForm");
var btnSubmit = document.getElementById("btnSubmit");
var formMessage = document.getElementById("formMessage");
var loginForm = document.getElementById("loginForm");
var loginBlock = document.getElementById("loginBlock");
var loginToggle = document.getElementById("loginToggle");
var formAuthInfo = document.getElementById("formAuthInfo");
var loginError = document.getElementById("loginError");

var messageTimer = null;
var currentAppId = null;

function initMobileMenu() {
    if (!mobileMenuToggle || !mobileMenu) return;
    var overlay = document.createElement("div");
    overlay.className = "mobile-menu-overlay";
    document.body.appendChild(overlay);
    function toggleMenu() {
        mobileMenuToggle.classList.toggle("active");
        mobileMenu.classList.toggle("active");
        overlay.classList.toggle("active");
        document.body.style.overflow = mobileMenu.classList.contains("active") ? "hidden" : "";
    }
    mobileMenuToggle.addEventListener("click", toggleMenu);
    overlay.addEventListener("click", toggleMenu);
    var links = mobileMenu.querySelectorAll("a");
    links.forEach(function(link) {
        link.addEventListener("click", function() {
            if (mobileMenu.classList.contains("active")) toggleMenu();
        });
    });
}

function clearFieldErrors() {
    document.querySelectorAll(".form-field").forEach(function(el) {
        el.classList.remove("field-has-error");
        var err = el.querySelector(".field-error");
        if (err) err.textContent = "";
    });
}

function showFieldError(fieldName, message) {
    var wrap = document.querySelector('.form-field[data-field="' + fieldName + '"]');
    if (!wrap) return;
    wrap.classList.add("field-has-error");
    var err = wrap.querySelector(".field-error");
    if (err) err.textContent = message;
}

function showMessage(text, isSuccess) {
    if (messageTimer) clearTimeout(messageTimer);
    if (!formMessage) return;
    formMessage.textContent = text;
    formMessage.className = "form-message " + (isSuccess ? "success" : "error");
    formMessage.style.display = "block";
    if (isSuccess) messageTimer = setTimeout(hideMessage, 10000);
}

function hideMessage() {
    if (messageTimer) clearTimeout(messageTimer);
    messageTimer = null;
    if (formMessage) {
        formMessage.className = "form-message";
        formMessage.style.display = "none";
    }
}

function validateForm() {
    clearFieldErrors();
    var errors = [];
    var fio = document.getElementById("fio").value.trim();
    if (!fio) {
        showFieldError("fio", "Поле обязательно.");
        errors.push("fio");
    } else if (fio.length > 150) {
        showFieldError("fio", "Не более 150 символов.");
        errors.push("fio");
    } else if (!FIO_REGEX.test(fio)) {
        showFieldError("fio", "Только буквы, пробелы и дефис.");
        errors.push("fio");
    }
    var phone = document.getElementById("phone").value.trim();
    if (!phone) {
        showFieldError("phone", "Поле обязательно.");
        errors.push("phone");
    } else if (phone.length > 30) {
        showFieldError("phone", "Не более 30 символов.");
        errors.push("phone");
    } else if (!PHONE_REGEX.test(phone)) {
        showFieldError("phone", "Только цифры и + - ( ).");
        errors.push("phone");
    }
    var email = document.getElementById("email").value.trim();
    if (!email) {
        showFieldError("email", "Поле обязательно.");
        errors.push("email");
    } else if (email.length > 255) {
        showFieldError("email", "Не более 255 символов.");
        errors.push("email");
    } else if (!EMAIL_REGEX.test(email)) {
        showFieldError("email", "Недопустимый формат e-mail.");
        errors.push("email");
    }
    var birthdate = document.getElementById("birthdate").value.trim();
    if (!birthdate) {
        showFieldError("birthdate", "Поле обязательно.");
        errors.push("birthdate");
    } else if (!DATE_REGEX.test(birthdate)) {
        showFieldError("birthdate", "Формат ГГГГ-ММ-ДД.");
        errors.push("birthdate");
    }
    var gender = (document.querySelector('input[name="gender"]:checked') || {}).value;
    if (!gender) {
        showFieldError("gender", "Выберите пол.");
        errors.push("gender");
    }
    var langSel = document.getElementById("languages");
    var selected = [].filter.call(langSel.options, function(o) { return o.selected; }).map(function(o) { return o.value; });
    if (selected.length === 0) {
        showFieldError("languages", "Выберите хотя бы один язык.");
        errors.push("languages");
    } else {
        var bad = selected.some(function(v) { return LANG_IDS.indexOf(v) === -1; });
        if (bad) {
            showFieldError("languages", "Недопустимый язык.");
            errors.push("languages");
        }
    }
    var bio = document.getElementById("bio").value.trim();
    if (bio.length > 5000) {
        showFieldError("bio", "Не более 5000 символов.");
        errors.push("bio");
    }
    var agreement = document.getElementById("agreement").checked;
    if (!agreement) {
        showFieldError("agreement", "Отметьте ознакомление с контрактом.");
        errors.push("agreement");
    }
    return errors.length === 0;
}

function getFormData() {
    var langSel = document.getElementById("languages");
    var languages = [].filter.call(langSel.options, function(o) { return o.selected; }).map(function(o) { return o.value; });
    return {
        fio: document.getElementById("fio").value.trim(),
        phone: document.getElementById("phone").value.trim(),
        email: document.getElementById("email").value.trim(),
        birthdate: document.getElementById("birthdate").value.trim(),
        gender: (document.querySelector('input[name="gender"]:checked') || {}).value || "",
        languages: languages,
        bio: document.getElementById("bio").value.trim(),
        agreement: document.getElementById("agreement").checked
    };
}

function setFormData(data) {
    if (!data) return;
    document.getElementById("fio").value = data.fio || "";
    document.getElementById("phone").value = data.phone || "";
    document.getElementById("email").value = data.email || "";
    document.getElementById("birthdate").value = data.birthdate || "";
    var gender = document.querySelector('input[name="gender"][value="' + (data.gender || "") + '"]');
    if (gender) gender.checked = true;
    var langSel = document.getElementById("languages");
    [].forEach.call(langSel.options, function(o) {
        o.selected = (data.languages || []).indexOf(o.value) !== -1;
    });
    document.getElementById("bio").value = data.bio || "";
    document.getElementById("agreement").checked = !!data.agreement;
}

function loadMe() {
    fetch(BASE + "/me", { credentials: "include" })
        .then(function(res) {
            if (res.status === 200) return res.json();
            return null;
        })
        .then(function(data) {
            if (data && data.fio !== undefined) {
                setFormData(data);
                currentAppId = data.id != null ? data.id : null;
                if (formAuthInfo) formAuthInfo.textContent = "Вы вошли. Можно редактировать данные и отправить форму для сохранения.";
                if (loginToggle) loginToggle.textContent = "Войти снова";
            } else {
                formAuthInfo.textContent = "";
                if (loginToggle) loginToggle.textContent = "Войти";
                currentAppId = null;
            }
        })
        .catch(function() {
            if (formAuthInfo) formAuthInfo.textContent = "";
            currentAppId = null;
        });
}

function submitFormAjax(e) {
    e.preventDefault();
    if (!validateForm()) {
        showMessage("Исправьте ошибки в форме.", false);
        document.getElementById("contacts").scrollIntoView({ behavior: "smooth" });
        return;
    }
    var data = getFormData();
    btnSubmit.disabled = true;
    btnSubmit.textContent = "Отправка...";
    clearFieldErrors();
    hideMessage();

    var url = BASE + "/";
    var method = "POST";
    if (currentAppId != null) {
        url = BASE + "/applications/" + currentAppId;
        method = "PUT";
    }
    fetch(url, {
        method: method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
        credentials: "include"
    })
        .then(function(res) {
            return res.json().then(function(body) {
                return { status: res.status, body: body };
            });
        })
        .then(function(result) {
            if (result.status === 200) {
                if (result.body.login !== undefined) {
                    showMessage("Данные сохранены. Ваш логин: " + result.body.login + ", пароль: " + result.body.password + ". Адрес профиля: " + (result.body.profile_url || BASE + "/"), true);
                } else {
                    showMessage("Данные обновлены.", true);
                }
                var msgEl = document.getElementById("formMessage");
                if (msgEl) msgEl.scrollIntoView({ block: "nearest" });
            } else if (result.status === 400 && result.body.errors) {
                var errs = result.body.errors;
                for (var k in errs) if (errs.hasOwnProperty(k) && k !== "_") showFieldError(k, errs[k]);
                showMessage(errs["_"] || "Ошибка валидации.", false);
            } else {
                showMessage(result.body.error || result.body._ || "Ошибка отправки.", false);
            }
        })
        .catch(function() {
            showMessage("Ошибка сети.", false);
        })
        .then(function() {
            if (btnSubmit) {
                btnSubmit.disabled = false;
                btnSubmit.textContent = "Отправить заявку";
            }
        });
}

function initLoginForm() {
    if (!loginToggle || !loginBlock) return;
    loginToggle.addEventListener("click", function(e) {
        e.preventDefault();
        loginBlock.style.display = loginBlock.style.display === "none" ? "block" : "none";
        loginError.textContent = "";
    });
    if (loginForm) {
        loginForm.addEventListener("submit", function(e) {
            e.preventDefault();
            var login = document.getElementById("login").value.trim();
            var password = document.getElementById("password").value;
            loginError.textContent = "";
            if (!login || !password) {
                loginError.textContent = "Введите логин и пароль.";
                return;
            }
            fetch(BASE + "/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ login: login, password: password }),
                credentials: "include"
            })
                .then(function(res) { return res.json().then(function(b) { return { status: res.status, body: b }; }); })
                .then(function(r) {
                    if (r.status === 200 && r.body.ok) {
                        loginBlock.style.display = "none";
                        document.getElementById("login").value = "";
                        document.getElementById("password").value = "";
                        loadMe();
                    } else {
                        loginError.textContent = r.body.error || "Неверный логин или пароль.";
                    }
                })
                .catch(function() {
                    loginError.textContent = "Ошибка сети.";
                });
        });
    }
}

function decodeBase64Url(s) {
    try {
        s = s.replace(/-/g, "+").replace(/_/g, "/");
        return decodeURIComponent(escape(atob(s)));
    } catch (e) {
        return s;
    }
}

function fallbackFromCookies() {
    var params = new URLSearchParams(window.location.search);
    if (params.get("from") === "error" || params.get("msg")) {
        var cookies = document.cookie.split(";");
        var msg = "";
        var errs = {};
        cookies.forEach(function(c) {
            c = c.trim();
            var eq = c.indexOf("=");
            if (eq === -1) return;
            var name = c.substring(0, eq).trim();
            var val = c.substring(eq + 1).trim();
            try { val = decodeURIComponent(val); } catch (e) {}
            if (name.indexOf("fa_err_") === 0) {
                errs[name.replace("fa_err_", "")] = decodeBase64Url(val);
            }
            if (name === "fa_msg" && val) {
                msg = decodeBase64Url(val);
            }
        });
        if (msg) {
            showMessage(msg, true);
            document.getElementById("contacts").scrollIntoView({ block: "nearest" });
        }
        for (var k in errs) showFieldError(k, errs[k]);
        if (Object.keys(errs).length) showMessage("Исправьте ошибки в форме.", false);
    }
}

function initSmoothScroll() {
    document.querySelectorAll('a[href^="#"]').forEach(function(link) {
        link.addEventListener("click", function(event) {
            var href = link.getAttribute("href");
            if (href === "#" || href === "") return;
            var target = document.querySelector(href);
            if (target) {
                event.preventDefault();
                target.scrollIntoView({ behavior: "smooth", block: "start" });
            }
        });
    });
}

function initFAQ() {
    document.querySelectorAll(".faq-item").forEach(function(item) {
        var question = item.querySelector(".faq-question");
        if (!question) return;
        question.addEventListener("click", function() {
            var isActive = item.classList.contains("faq-active");
            document.querySelectorAll(".faq-item").forEach(function(i) { i.classList.remove("faq-active"); });
            if (!isActive) item.classList.add("faq-active");
        });
    });
}

function initReviewsCarousel() {
    var prevBtn = document.getElementById("prevReview");
    var nextBtn = document.getElementById("nextReview");
    if (!prevBtn || !nextBtn) return;
    var currentReview = 0;
    var reviews = [
        { logo: "img/logo.png", text: '"Отличная команда профессионалов! Работа выполнена качественно и в срок."', author: "— Компания CIEL" },
        { logo: "img/cableman_ru.png", text: '"Отличная команда профессионалов! Работа выполнена качественно и в срок."', author: "— Компания Cableman" },
        { logo: "img/farbors_ru.jpg", text: '"Отличная команда профессионалов! Работа выполнена качественно и в срок."', author: "— Компания Farbors" },
        { logo: "img/nashagazeta_ch.png", text: '"Отличная команда профессионалов! Работа выполнена качественно и в срок."', author: "— Компания Наша газета" },
        { logo: "img/lpcma_rus_v4.jpg", text: '"Отличная команда профессионалов! Работа выполнена качественно и в срок."', author: "— Компания LPCMA" }
    ];
    function updateReview() {
        var card = document.querySelector(".review-card");
        if (card && reviews[currentReview]) {
            var r = reviews[currentReview];
            var img = card.querySelector(".review-logo img");
            if (img) img.src = r.logo;
            var text = card.querySelector(".review-text");
            if (text) text.textContent = r.text;
            var author = card.querySelector(".review-author");
            if (author) author.textContent = r.author;
        }
    }
    updateReview();
    prevBtn.addEventListener("click", function() {
        currentReview = (currentReview - 1 + reviews.length) % reviews.length;
        updateReview();
    });
    nextBtn.addEventListener("click", function() {
        currentReview = (currentReview + 1) % reviews.length;
        updateReview();
    });
}

function initHeroButton() {
    var heroBtn = document.getElementById("heroTariffsBtn");
    if (heroBtn) {
        heroBtn.addEventListener("click", function() {
            var section = document.getElementById("tariffs");
            if (section) section.scrollIntoView({ behavior: "smooth", block: "start" });
        });
    }
}

document.addEventListener("DOMContentLoaded", function() {
    initMobileMenu();
    initSmoothScroll();
    initFAQ();
    initReviewsCarousel();
    initHeroButton();
    initLoginForm();
    fallbackFromCookies();
    loadMe();

    if (contactForm) {
        contactForm.setAttribute("action", BASE + "/");
        contactForm.setAttribute("method", "POST");
        contactForm.addEventListener("submit", submitFormAjax);
    }
});
