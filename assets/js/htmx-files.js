htmx.defineExtension("new-files", {
	onEvent: (name, event) => {
		if (name === "htmx:configRequest") {
			event.detail.path = event.detail.parameters.dir;
			event.detail.useUrlParams = true;
			event.detail.parameters = {
				dir: event.detail.parameters.dir,
				action: "new-files",
				files: event.detail.parameters.files.map(file => file.name).join(",")
			};
		}
	}
});

htmx.defineExtension("new-permissions", {
	onEvent: (name, event) => {
		if (name === "htmx:configRequest") {
			event.detail.path = event.detail.parameters.dir;
			event.detail.useUrlParams = true;
			const file = event.detail.parameters.file;
			event.detail.parameters = {
				dir: event.detail.parameters.dir,
				action: "new-permissions",
				file: file,
				index: event.detail.parameters.index,
				object_type: event.detail.parameters[`object_type-${file}`],
				object_name: event.detail.parameters[`object_name-${file}`]
			};
		}
	}
});

htmx.defineExtension("upload-files", {
	encodeParameters: (xhr, params, element) => {
		const data = new FormData();
		const files = [];
		for (let i = 0; i < params.files.length; i++) {
			const file = params.files[i];
			files.push({
				name: params[`name-${i}`] || file.name,
				description: params[`description-${i}`],
				overwrite: params[`overwrite-${i}`] === "true",
				size: file.size,
				permissions: parsePermissions(i, params)
			});
			data.set("json", JSON.stringify(files));
			data.append(`file-${i}`, file, params[`name-${i}`] || file.name);
		}
		return data;
	},
	onEvent: (name, event) => {
		if (name === "htmx:configRequest") {
			event.detail.path = event.detail.parameters.dir;
		}
	}
});

htmx.defineExtension("edit-file", {
	encodeParameters: (xhr, params, element) => {
		const data = new FormData();
		const file = params.files[0];

		const json = {
			dir: params["dir"],
			name: params["name-0"] || file.name,
			description: params["description-0"],
			permissions: parsePermissions(0, params),
		};

		if (file) {
			json.size = file.size;
		}

		data.append("json", JSON.stringify(json));
		if (file) {
			data.append("file", file, json.name);
		}

		return data;
	},
	onEvent: (name, event) => {
		if (name === "htmx:configRequest") {

		}
	}
});

htmx.defineExtension("edit-folder", {
	encodeParameters: (xhr, params, element) => {
		return undefined;
	},
	onEvent: (name, event) => {
		if (name === "htmx:configRequest") {
			event.detail.headers["Destination"] = event.detail.parameters.dir;
		}
	}
});

function parsePermissions(file, params) {
	const permissions = [];
	for (const permission of Object.keys(params)) {
		if (!permission.startsWith(`permissions-${file}`)) {
			continue;
		}
		const values = permission.split("-")
		const index = values[2]
		if (permissions.findIndex(p => p.index === index) !== -1) {
			continue;
		}
		permissions.push({
			index: index,
			object_type: +params[`permissions-${file}-${index}-object_type`],
			object: params[`permissions-${file}-${index}-object`],
			permissions: {
				create: +params[`permissions-${file}-${index}-create`],
				update: +params[`permissions-${file}-${index}-update`],
				delete: +params[`permissions-${file}-${index}-delete`],
				update_permissions: +params[`permissions-${file}-${index}-update_permissions`],
				read: +params[`permissions-${file}-${index}-read`],
			}
		});
	}
	return permissions;
}