package types

import (
	"encoding/xml"
	"time"
)

type ESDoc struct {
	Timestamp  time.Time `json:"timestamp"`
	Cluster    string    `json:"cluster"`
	VolumeType string    `json:"volume_type"`
	FullPath   string    `json:"full_path"`
	DiskSize   string    `json:"disk_size"`
}

type ESDocGlobal struct {
	ESDoc
	VolumeName string `json:"volume_name"`
}

type ESDocProject struct {
	ESDocGlobal
	ProjectName string `json:"project_name"`
}

type ESDocPersonal struct {
	ESDoc
	UserName string `json:"user_name"`
}

type DUC_RET struct {
	XMLNmae xml.Name `xml:"duc"`
	Root    string   `xml:"root,attr"`
	Size    string   `xml:"size_actual,attr"`
}
