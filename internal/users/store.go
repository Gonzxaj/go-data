// Package users persists dashboard accounts and their login sessions in a
// SQLite file under the data dir. It's only used when AUTH_MODE=session.
package users

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite" // pure-Go driver: keeps CGO_ENABLED=0 builds working
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

func (r Role) Valid() bool { return r == RoleAdmin || r == RoleViewer }

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	ErrNotFound      = errors.New("usuario no encontrado")
	ErrUsernameTaken = errors.New("el nombre de usuario ya existe")
	ErrLastAdmin     = errors.New("debe quedar al menos un admin activo")
	ErrBadUsername   = errors.New("usuario inválido: 3 a 32 caracteres (letras, números, . _ -)")
	ErrBadPassword   = errors.New("la contraseña debe tener entre 8 y 72 caracteres")
	ErrBadRole       = errors.New("rol inválido: admin o viewer")
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL CHECK (role IN ('admin','viewer')),
	active        INTEGER NOT NULL DEFAULT 1,
	created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions(user_id);
`

type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection serialises writes, which sidesteps SQLITE_BUSY;
	// traffic here is a handful of dashboard users, so it's never a bottleneck.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) List() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, role, active, created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) Get(id int64) (User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT id, username, role, active, created_at FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) Create(username, password string, role Role) (User, error) {
	if !usernameRe.MatchString(username) {
		return User{}, ErrBadUsername
	}
	if !role.Valid() {
		return User{}, ErrBadRole
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&exists); err != nil {
		return User{}, err
	}
	if exists > 0 {
		return User{}, ErrUsernameTaken
	}
	now := time.Now()
	res, err := tx.Exec(`INSERT INTO users (username, password_hash, role, active, created_at) VALUES (?, ?, ?, 1, ?)`,
		username, hash, string(role), now.Unix())
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return User{ID: id, Username: username, Role: role, Active: true, CreatedAt: time.Unix(now.Unix(), 0)}, nil
}

// Update is a partial update: nil fields are left untouched. Deactivating a
// user or changing their password drops all their sessions except
// keepSession (a token hash, "" for none), so the change takes effect at once.
type Update struct {
	Role     *Role
	Active   *bool
	Password *string
}

func (s *Store) Update(id int64, upd Update, keepSession string) (User, error) {
	if upd.Role != nil && !upd.Role.Valid() {
		return User{}, ErrBadRole
	}
	var hash string
	if upd.Password != nil {
		h, err := hashPassword(*upd.Password)
		if err != nil {
			return User{}, err
		}
		hash = h
	}

	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	cur, err := scanUser(tx.QueryRow(`SELECT id, username, role, active, created_at FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}

	next := cur
	if upd.Role != nil {
		next.Role = *upd.Role
	}
	if upd.Active != nil {
		next.Active = *upd.Active
	}
	losesAdmin := cur.Role == RoleAdmin && cur.Active && (next.Role != RoleAdmin || !next.Active)
	if losesAdmin {
		if err := ensureOtherAdmin(tx, id); err != nil {
			return User{}, err
		}
	}

	if _, err := tx.Exec(`UPDATE users SET role = ?, active = ? WHERE id = ?`, string(next.Role), next.Active, id); err != nil {
		return User{}, err
	}
	if upd.Password != nil {
		if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id); err != nil {
			return User{}, err
		}
	}
	if upd.Password != nil || !next.Active {
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, id, keepSession); err != nil {
			return User{}, err
		}
	}
	return next, tx.Commit()
}

func (s *Store) Delete(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	cur, err := scanUser(tx.QueryRow(`SELECT id, username, role, active, created_at FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if cur.Role == RoleAdmin && cur.Active {
		if err := ensureOtherAdmin(tx, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// Authenticate returns the active user matching username/password. It always
// runs a bcrypt comparison, even for unknown users, so response time doesn't
// reveal which usernames exist.
func (s *Store) Authenticate(username, password string) (User, bool) {
	var hash string
	u, err := scanUserWith(s.db.QueryRow(
		`SELECT id, username, role, active, created_at, password_hash FROM users WHERE username = ?`, username), &hash)
	if err != nil || !u.Active {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, false
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, false
	}
	return u, true
}

// VerifyPassword checks the current password of user id.
func (s *Store) VerifyPassword(id int64, password string) bool {
	var hash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash); err != nil {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ---------- sessions ----------

// CreateSession issues a random token for userID. The raw token goes to the
// client cookie; only its SHA-256 is stored, so a leaked DB can't be replayed.
func (s *Store) CreateSession(userID int64, ttl time.Duration) (token string, expires time.Time, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	expires = time.Now().Add(ttl)
	_, err = s.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		HashToken(token), userID, expires.Unix())
	return token, expires, err
}

// SessionUser resolves a token hash to its (active) user. Role and active
// flag are read fresh on every request, so admin changes apply immediately.
func (s *Store) SessionUser(tokenHash string) (User, bool) {
	u, err := scanUser(s.db.QueryRow(`
		SELECT u.id, u.username, u.role, u.active, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND u.active = 1`, tokenHash, time.Now().Unix()))
	return u, err == nil
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) PurgeExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	return err
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---------- helpers ----------

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)

func hashPassword(pw string) (string, error) {
	if len(pw) < 8 || len(pw) > 72 { // bcrypt ignores bytes past 72
		return "", ErrBadPassword
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(h), err
}

func ensureOtherAdmin(tx *sql.Tx, exceptID int64) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND active = 1 AND id != ?`, exceptID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrLastAdmin
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (User, error) { return scanUserWith(row) }

func scanUserWith(row scanner, extra ...any) (User, error) {
	var u User
	var role string
	var created int64
	dest := append([]any{&u.ID, &u.Username, &role, &u.Active, &created}, extra...)
	if err := row.Scan(dest...); err != nil {
		return User{}, err
	}
	u.Role = Role(role)
	u.CreatedAt = time.Unix(created, 0)
	return u, nil
}
