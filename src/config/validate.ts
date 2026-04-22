import type { Config } from "./schema.js";

const SUPPORTED_TRIGGER_CONNECTORS = ["linear"];
const SUPPORTED_NOTIFICATION_CONNECTORS = ["linear"];

export interface ValidationError {
	automation: string;
	field: string;
	message: string;
}

/**
 * Validate that the config is internally consistent:
 * - Automation trigger connectors reference defined connectors
 * - Automation notification connectors reference defined connectors
 * - Trigger connector types are supported
 * - Notification connector types are supported
 * - Automation names are unique
 *
 * Returns an array of validation errors (empty if valid).
 */
export function validateConfig(config: Config): ValidationError[] {
	const errors: ValidationError[] = [];
	const connectorNames = new Set(Object.keys(config.connectors));
	const automationNames = new Set<string>();

	for (const automation of config.automations) {
		// Check for duplicate automation names
		if (automationNames.has(automation.name)) {
			errors.push({
				automation: automation.name,
				field: "name",
				message: `Duplicate automation name "${automation.name}"`,
			});
		}
		automationNames.add(automation.name);

		// Validate trigger connector exists
		const triggerConnector = automation.trigger.connector;
		if (!connectorNames.has(triggerConnector)) {
			errors.push({
				automation: automation.name,
				field: "trigger.connector",
				message: `Trigger references connector "${triggerConnector}" which is not defined in connectors`,
			});
		}

		// Validate trigger connector type is supported
		if (!SUPPORTED_TRIGGER_CONNECTORS.includes(triggerConnector)) {
			errors.push({
				automation: automation.name,
				field: "trigger.connector",
				message: `Trigger connector type "${triggerConnector}" is not supported (supported: ${SUPPORTED_TRIGGER_CONNECTORS.join(", ")})`,
			});
		}

		// Validate notification connectors
		for (const notification of automation.notifications) {
			if (!connectorNames.has(notification.connector)) {
				errors.push({
					automation: automation.name,
					field: "notifications.connector",
					message: `Notification references connector "${notification.connector}" which is not defined in connectors`,
				});
			}

			if (!SUPPORTED_NOTIFICATION_CONNECTORS.includes(notification.connector)) {
				errors.push({
					automation: automation.name,
					field: "notifications.connector",
					message: `Notification connector type "${notification.connector}" is not supported (supported: ${SUPPORTED_NOTIFICATION_CONNECTORS.join(", ")})`,
				});
			}
		}
	}

	return errors;
}

/**
 * Validate config and throw with a formatted error message if invalid.
 */
export function assertConfigValid(config: Config): void {
	const errors = validateConfig(config);
	if (errors.length > 0) {
		const formatted = errors
			.map((e) => `  - [${e.automation}] ${e.field}: ${e.message}`)
			.join("\n");
		throw new Error(`Configuration validation failed:\n${formatted}`);
	}
}
