local dap = require("dap")

dap.configurations.go = {
	{
		type = "go",
		name = "Debug operator",
		request = "launch",
		program = vim.g.project_root_dir .. "./cmd",
		args = {},
		console = "externalTerminal",
		-- env = {
		-- 	["HELM_JOB_IMAGE"] = "ghcr.io/kloudlite/kloudlite/operator/workers/helm-job-runner:v1.1.4",
		-- },
	},
}
