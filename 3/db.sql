CREATE TABLE programming_language (
  id int(10) unsigned NOT NULL,
  name varchar(50) NOT NULL DEFAULT '',
  PRIMARY KEY (id)
);

CREATE TABLE application (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  fio varchar(150) NOT NULL DEFAULT '',
  phone varchar(30) NOT NULL DEFAULT '',
  email varchar(255) NOT NULL DEFAULT '',
  birthdate date NOT NULL,
  gender varchar(10) NOT NULL DEFAULT '',
  biography text,
  contract_agreed int(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
);

CREATE TABLE application_language (
  application_id int(10) unsigned NOT NULL,
  language_id int(10) unsigned NOT NULL,
  PRIMARY KEY (application_id, language_id)
);

INSERT INTO programming_language (id, name) VALUES
(1, 'Pascal'),
(2, 'C'),
(3, 'C++'),
(4, 'JavaScript'),
(5, 'PHP'),
(6, 'Python'),
(7, 'Java'),
(8, 'Haskel'),
(9, 'Clojure'),
(10, 'Prolog'),
(11, 'Scala'),
(12, 'Go');
