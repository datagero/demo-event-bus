package config

// Config holds the configuration for the API server
type Config struct {
	Port        string `json:"port"`
	PythonURL   string `json:"python_url"`
	WorkersURL  string `json:"workers_url"`
	RabbitMQURL string `json:"rabbitmq_url"`
}
