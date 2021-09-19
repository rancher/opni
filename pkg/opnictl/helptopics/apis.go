package helptopics

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
)

var ApisHelpCmd = &cobra.Command{
	Use: "apis",
	Long: fmt.Sprintf(`
The Opni Manager recognizes two separate APIs and either can be used to install
Opni into your cluster. 

%s
The Opni Demo API (demo.opni.io/v1alpha1) is used for demonstration purposes 
to showcase the features of Opni and is not intended for production use. 

Opni can be installed using the Demo API with the command %s. 
The installation can be customized with a variety of options. For a list of
options, run %s.

%s
The Opni Production API (opni.io/v1beta1) is used for production deployments of Opni.
It is more feature-rich and configurable than the Demo API and will have long-term
development and support. 
The Production API should be preferred when installing Opni. The Demo API may
be deprecated or removed in the future.

Opni can be installed using the Production API with the command %s.
The installation can be customized with a variety of options. For a list of
options, run %s.
`,
		chalk.Bold.TextStyle("Demo API"),
		chalk.Bold.TextStyle("opnictl create demo"),
		chalk.Bold.TextStyle("opnictl help create demo"),
		chalk.Bold.TextStyle("Production API "),
		chalk.Bold.TextStyle("opnictl create cluster"),
		chalk.Bold.TextStyle("opnictl help create cluster"),
	),
}
