import { drizzle } from "drizzle-orm/postgres-js";
import { migrate } from "drizzle-orm/postgres-js/migrator";
import postgres from "postgres";
import { logger } from "../middleware/logger.js";

/**
 * Run pending Drizzle migrations against the database.
 * Safe to call on every startup — already-applied migrations are skipped.
 */
export async function runMigrations(connectionString?: string) {
	const url = connectionString ?? process.env.DATABASE_URL;
	if (!url) {
		throw new Error("DATABASE_URL environment variable is required");
	}

	// Use a separate connection for migrations (max 1) so we don't interfere
	// with the main connection pool.
	const client = postgres(url, { max: 1 });
	const db = drizzle(client);

	try {
		logger.info("Running database migrations...");
		await migrate(db, { migrationsFolder: "drizzle" });
		logger.info("Database migrations complete");
	} finally {
		await client.end();
	}
}
