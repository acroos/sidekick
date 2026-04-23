CREATE TABLE IF NOT EXISTS "run_notifications" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"run_id" uuid NOT NULL,
	"connector" text NOT NULL,
	"target_id" text NOT NULL,
	"target_url" text,
	"config" jsonb,
	"status" text DEFAULT 'pending' NOT NULL,
	"error" text,
	"retry_count" integer DEFAULT 0 NOT NULL,
	"max_retries" integer DEFAULT 3 NOT NULL,
	"notified_at" timestamp with time zone,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "runs" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"automation" text NOT NULL,
	"trigger_type" text NOT NULL,
	"trigger_id" text NOT NULL,
	"trigger_url" text,
	"github_run_id" text,
	"repo" text NOT NULL,
	"status" text DEFAULT 'triggered' NOT NULL,
	"context" jsonb,
	"result" jsonb,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL,
	"updated_at" timestamp with time zone DEFAULT now() NOT NULL,
	"completed_at" timestamp with time zone
);
--> statement-breakpoint
DO $$ BEGIN
  ALTER TABLE "run_notifications" ADD CONSTRAINT "run_notifications_run_id_runs_id_fk" FOREIGN KEY ("run_id") REFERENCES "public"."runs"("id") ON DELETE no action ON UPDATE no action;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;