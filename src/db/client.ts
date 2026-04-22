import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import * as schema from "./schema.js";

export function createDb(connectionString?: string) {
	const url = connectionString ?? process.env.DATABASE_URL;
	if (!url) {
		throw new Error("DATABASE_URL environment variable is required");
	}
	const client = postgres(url);
	return drizzle(client, { schema });
}

export type Database = ReturnType<typeof createDb>;
