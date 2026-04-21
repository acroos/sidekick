# Sidekick

The idea for this system is to allow users to send a message (likely that's just a UI that results in an API call to the core service here) that triggers an agent to autonomously work on a task. Think of a system like Stripe's minions (read more [here](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents) and part two [here](https://stripe.dev/blog/minions-stripes-one-shot-end-to-end-coding-agents-part-2)). Some key components from this are:

1. Safely sandboxing agents so they cannot do any real harm
2. Separating agentic workflows from deterministic workflows, and having both as part of task completion
3. Avoiding infinite loops, an agent should try a task for a constrained amount of time before giving up

I envision this as OSS that organizations can run on their own infrastructure more than a managed service.

The system should have a simple API which can easily be built on top of. I think teams/orgs will end up building slack bots, web UIs, github apps, or many other things which can call this API.
