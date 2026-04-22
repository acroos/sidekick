import { Hono } from "hono";
import { healthRoutes } from "./routes/health.js";

const app = new Hono();

app.route("/api", healthRoutes);

export default app;
