package skill

type Type string

const (
	TypeCode      Type = "code"
	TypeRunbook   Type = "runbook"
	TypeInfra     Type = "infra"
	TypeProto     Type = "proto"
	TypeTerraform Type = "terraform"
	TypeDoc       Type = "doc"
)

type Skill struct {
	Name         string
	Type         Type
	Source       string
	Tags         []string
	Dependencies []string
	Summary      string
	Pattern      string
	Usage        string
}
