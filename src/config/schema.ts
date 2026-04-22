import { z } from "zod";

const connectorConfigSchema = z.object({
	api_key: z.string().optional(),
	webhook_secret: z.string().optional(),
	bot_token: z.string().optional(),
});

const triggerSchema = z.object({
	connector: z.string(),
	on_label: z.string().optional(),
	on_reaction: z.string().optional(),
	channel: z.string().optional(),
	context: z
		.object({
			include: z.array(z.string()).optional(),
		})
		.optional(),
});

const notificationSchema = z.object({
	connector: z.string(),
	comment: z.boolean().optional(),
	update_status: z.boolean().optional(),
	status_mapping: z.record(z.string()).optional(),
	thread_reply: z.boolean().optional(),
	create_issue: z.boolean().optional(),
	team: z.string().optional(),
	channel: z.string().optional(),
	on: z.array(z.string()).optional(),
});

const automationSchema = z.object({
	name: z.string(),
	trigger: triggerSchema,
	notifications: z.array(notificationSchema).default([]),
	repo: z.string().optional(),
});

const githubConfigSchema = z.object({
	token: z.string(),
	default_repo: z.string(),
	workflow: z.string(),
});

export const configSchema = z.object({
	github: githubConfigSchema,
	connectors: z.record(connectorConfigSchema).default({}),
	automations: z.array(automationSchema).default([]),
});

export type Config = z.infer<typeof configSchema>;
export type Automation = z.infer<typeof automationSchema>;
export type Trigger = z.infer<typeof triggerSchema>;
export type Notification = z.infer<typeof notificationSchema>;
