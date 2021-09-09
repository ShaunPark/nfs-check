package types

type Config struct {
	Days          []DayConfig   `yaml:"jobs"`
	MountDir      string        `yaml:"mountDir"`
	OutputDir     string        `yaml:"outputDir"`
	ElasticSearch ElasticSearch `yaml:"elasticSearch"`
}

type DayConfig struct {
	Day     string   `yaml:"day"`
	Targets []Target `yaml:"targets"`
}

type Target struct {
	Type     string   `yaml:"type"`
	Location string   `yaml:"location"`
	JobType  string   `yaml:"jobType"`
	SkipDirs []string `yaml:"skipDirs"`
}

type ElasticSearch struct {
	Host      string `yaml:"host"`
	Port      string `yaml:"port"`
	Id        string `yaml:"id"`
	IndexName string `yaml:"indexName"`
}
