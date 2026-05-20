package aidevengine

import "context"

const displayName = "AI 开发引擎"

type StatusResponse struct {
	Status      string `json:"status"`
	AuthStatus  string `json:"authStatus"`
	DisplayName string `json:"displayName"`
}

type Capability struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type CapabilitiesResponse struct {
	DisplayName  string       `json:"displayName"`
	Capabilities []Capability `json:"capabilities"`
}

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Status(ctx context.Context) StatusResponse {
	_ = ctx
	return StatusResponse{
		Status:      "disabled",
		AuthStatus:  "not_configured",
		DisplayName: displayName,
	}
}

func (s *Service) Capabilities(ctx context.Context) CapabilitiesResponse {
	_ = ctx
	return CapabilitiesResponse{
		DisplayName: displayName,
		Capabilities: []Capability{
			{Key: "read_project", Name: "读取项目", Enabled: false},
			{Key: "modify_files", Name: "修改文件", Enabled: false},
			{Key: "execute_commands", Name: "执行命令", Enabled: false},
			{Key: "browser_operations", Name: "浏览器操作", Enabled: false},
			{Key: "git_operations", Name: "Git 操作", Enabled: false},
		},
	}
}
