package config

// Config holds the configuration for the API server
type Config struct {
	Port        string `json:"port"`
	WorkersURL  string `json:"workers_url"`
	RabbitMQURL string `json:"rabbitmq_url"`
}
