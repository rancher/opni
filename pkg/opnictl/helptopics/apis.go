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
The Opni Demo API is used for demonstration purposes to showcase the features
of Opni without needing a complete multi-cluster setup. This API should not be
used for production purposes.

Opni can be installed using the Demo API with the command %s. 
The installation can be customized with a variety of options. For a list of
options, run %s.

%s
The Opni Production API is used for production deployments of Opni. 

%s
`,
		chalk.Bold.TextStyle("Demo API"),
		chalk.Bold.TextStyle("opnictl create demo"),
		chalk.Bold.TextStyle("opnictl help install demo"),
		chalk.Bold.TextStyle("Production API ")+chalk.Blue.Color("(Coming Soon)"),
		chalk.Yellow.Color(`The Production API is not available yet. Check the github page at 
https://github.com/rancher/opni for more information on upcoming releases.`),
	),
}
