# Backend Home Assignment — Expression Evaluation Engine

*This is the original take-home prompt, reproduced as markdown for reference.*

**The task:** Build a library that evaluates expressions like `2 + 3 * (x - 1)` against a provided
set of named variables. Picture our product letting data scientists define computed variables and
logics using these expressions. Feel more than free to use Claude and/or other AI tools for this
assignment.

## Context — please read carefully

- This engine is going to run **millions of times per second** in production, usually for similar
  expressions.
- We plan to **open-source** it once it's mature.
- Other backend teams in the company will pull it as a **dependency**.
- Make sure we support **booleans and conditional operators** like: ternary if, and, or, etc.
- Make sure expressions can also handle **strings**.
- Allow to **define and use variables** inside expressions.
- Support `+`, `-`, `*`, `/`, **parentheses**, and **variable lookup**. We'd also like common math
  operations available.
- Support **defining and using functions** inside expressions.
- It should be **thread-safe**.
- **Null and undefined** values are handled throughout evaluation.

## What to submit

- **Working code.** Choose from either Python or Go.
- **A short write-up (max 1 page):**
  - How did you approach this? What did you decide vs. what did the AI decide? Some instructions are
    vague by design — tell us what you decided and why.
  - Describe the library. How to use it, and what kind of features and expressions it supports.
- **Your AI chat transcripts.** Whatever you used. If your tool stores them locally (e.g. a CLI
  agent), share whatever you have.

## Design answer (½ page max)

A data scientist complains: *"for some reason my expressions always evaluate to 0 but it does not
make any sense, what's going on?"* Put on a product manager hat. What feature would you propose to
help them? Don't build it — just write the proposal.
