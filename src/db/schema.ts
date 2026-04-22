import {
	integer,
	jsonb,
	pgTable,
	text,
	timestamp,
	uuid,
} from "drizzle-orm/pg-core";

export const runs = pgTable("runs", {
	id: uuid("id").primaryKey().defaultRandom(),
	automation: text("automation").notNull(),
	triggerType: text("trigger_type").notNull(),
	triggerId: text("trigger_id").notNull(),
	triggerUrl: text("trigger_url"),
	githubRunId: text("github_run_id"),
	repo: text("repo").notNull(),
	status: text("status").notNull().default("triggered"),
	context: jsonb("context"),
	result: jsonb("result"),
	createdAt: timestamp("created_at", { withTimezone: true })
		.notNull()
		.defaultNow(),
	updatedAt: timestamp("updated_at", { withTimezone: true })
		.notNull()
		.defaultNow(),
	completedAt: timestamp("completed_at", { withTimezone: true }),
});

export const runNotifications = pgTable("run_notifications", {
	id: uuid("id").primaryKey().defaultRandom(),
	runId: uuid("run_id")
		.notNull()
		.references(() => runs.id),
	connector: text("connector").notNull(),
	targetId: text("target_id").notNull(),
	targetUrl: text("target_url"),
	config: jsonb("config"),
	status: text("status").notNull().default("pending"),
	error: text("error"),
	retryCount: integer("retry_count").notNull().default(0),
	maxRetries: integer("max_retries").notNull().default(3),
	notifiedAt: timestamp("notified_at", { withTimezone: true }),
	createdAt: timestamp("created_at", { withTimezone: true })
		.notNull()
		.defaultNow(),
});
