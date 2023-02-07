/*
Package etcd implements data storage using etcd.

Key layout:

<prefix>/
├── tokens/
│   └── <id>: core.BootstrapToken
├── clusters/
│   └── <id>: core.Cluster
├── roles/
│   └── <id>: core.Role
├── rolebindings/
│   └── <id>: core.RoleBinding
├── keyrings/
│   └── <cluster-id>/
│       └── <capability-id>: keyring.Keyring
│── kv/
│	├── <prefix1>/
│	│   └── ...
│	└── <prefix2>/
│	    └── ...
*/
package etcd
