package types

var (
	Version    = "dev"
	CommitHash = "unknown"
	BuildDate  = "unknown"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Auth     AuthConfig     `json:"auth"`
	LLM      LLMConfig      `json:"llm"`
	Discovery DiscoveryConfig `json:"discovery"`
}

type ServerConfig struct {
	Addr            string `json:"addr"`
	CORSOrigin      string `json:"cors_origin"`
	ReadTimeout     int    `json:"read_timeout_seconds"`
	WriteTimeout    int    `json:"write_timeout_seconds"`
	ShutdownTimeout int    `json:"shutdown_timeout_seconds"`
}

type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"-"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"`
	MaxConns int    `json:"max_conns"`
}

func (c *DatabaseConfig) DSN() string {
	return "host=" + c.Host +
		" port=" + itoa(c.Port) +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.Database +
		" sslmode=" + c.SSLMode
}

type AuthConfig struct {
	JWTPrivateKeyPath string `json:"jwt_private_key_path"`
	EncryptionKey     string `json:"-"`
	AccessTokenTTL    int    `json:"access_token_ttl_minutes"`
	RefreshTokenTTL   int    `json:"refresh_token_ttl_days"`
	Disabled          bool   `json:"disabled"`
}

type LLMConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIKey   string `json:"-"`
	BaseURL  string `json:"base_url"`
}

type DiscoveryConfig struct {
	PluginDir    string `json:"plugin_dir"`
	ScanInterval string `json:"scan_interval"`
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
