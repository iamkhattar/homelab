package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConnection(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		host    string
		want    Connection
		wantErr string
	}{
		{
			name: "inherits group variables and applies host overrides",
			yaml: `
k3s_cluster:
  children:
    server:
      vars:
        ansible_user: operator
        ansible_port: "2200"
      hosts:
        titan:
          ansible_host: 192.168.1.163
          ansible_port: 2222
          ansible_ssh_private_key_file: /keys/titan
  vars:
    ansible_port: 22
`,
			host: "titan",
			want: Connection{Address: "192.168.1.163", User: "operator", Port: 2222, IdentityFile: "/keys/titan"},
		},
		{
			name: "supports conventional all root",
			yaml: `
all:
  children:
    k3s_cluster:
      children:
        server:
          hosts:
            titan: {}
  vars:
    ansible_user: operator
`,
			host: "titan",
			want: Connection{Address: "titan", User: "operator", Port: 22},
		},
		{name: "missing host", yaml: "server:\n  hosts: {}\n", host: "titan", wantErr: "was not found"},
		{name: "empty inventory", yaml: "---\n", host: "titan", wantErr: "is empty"},
		{name: "invalid yaml", yaml: "server: [\n", host: "titan", wantErr: "parsing private inventory"},
		{
			name: "multiple group membership with compatible values",
			yaml: `
home:
  hosts:
    titan:
      ansible_host: titan.home
server:
  hosts:
    titan: {}
  vars:
    ansible_user: operator
`,
			host: "titan",
			want: Connection{Address: "titan.home", User: "operator", Port: 22},
		},
		{
			name:    "conflicting group values",
			yaml:    "one:\n  hosts:\n    titan:\n      ansible_user: one\ntwo:\n  hosts:\n    titan:\n      ansible_user: two\n",
			host:    "titan",
			wantErr: "conflicting ansible_user",
		},
		{
			name:    "example user",
			yaml:    "server:\n  hosts:\n    titan:\n      ansible_user: change-me\n",
			host:    "titan",
			wantErr: "example ansible_user",
		},
		{
			name:    "unsafe address",
			yaml:    "server:\n  hosts:\n    titan:\n      ansible_host: '-oProxyCommand=bad command'\n",
			host:    "titan",
			wantErr: "unsafe SSH value",
		},
		{
			name:    "invalid port",
			yaml:    "server:\n  hosts:\n    titan:\n      ansible_port: 70000\n",
			host:    "titan",
			wantErr: "outside 1-65535",
		},
		{
			name:    "invalid variable type",
			yaml:    "server:\n  hosts:\n    titan:\n      ansible_user: [operator]\n",
			host:    "titan",
			wantErr: "ansible_user must be a string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hosts.yml")
			if err := os.WriteFile(path, []byte(test.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := ResolveConnection(path, test.host)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ResolveConnection() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveConnection() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("ResolveConnection() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestResolveConnectionReportsMissingInventory(t *testing.T) {
	_, err := ResolveConnection(filepath.Join(t.TempDir(), "hosts.yml"), "titan")
	if err == nil || !strings.Contains(err.Error(), "reading private inventory") {
		t.Fatalf("ResolveConnection() error = %v, want missing inventory error", err)
	}
}
