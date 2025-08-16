# Scenarios

## 1) Late-bind Escort (with unreachable message)

**Goal:**
Prove that a late-joining worker can safely take over and drain an existing backlog from the shared escort queue, including a message sent before any worker existed.

**Steps (short & descriptive):**

1. **Start clean.** Reset game state and ensure the escort queue is empty.
2. **Send an escort message before any worker exists.** It cannot be consumed and remains **unreachable**.
3. **Start a temporary worker** and send a few more escort messages. The worker processes these successfully.
4. **Stop the worker** to simulate downtime. Any further messages **accumulate as backlog**.
5. **Wait briefly (delay).** Backlog count grows as expected.
6. **Start a new worker (late binder).** The new worker takes over, consumes the backlog, **and also picks up the initial unreachable message**.

**Expected:**

* Clean slate confirmed at the start.
* Unreachable message persists until a worker is available.
* Backlog accumulates while no worker is running.
* New worker drains both backlog and the initial unreachable message.
* Final queue ends clean (no stuck messages).

---

## 2) Reissuing DLQ (unroutable + failed → reissue)

**Goal:**
Validate that unroutable and failed messages can be reissued and successfully processed once the system is in a healthy state.

**Steps (short & descriptive):**

1. **Start clean.**
2. **Issue one `gather` message when there is no character or queue.** Record its **message ID**.
3. **Confirm** this message is **unroutable** (lands in DLQ/unroutable store).
4. **Create character “alice.”**
5. **Send three `gather` messages.** Force outcomes deterministically: **pass**, **fail**, **pass**.
6. **Reissue the failed message.** The **same message ID** is now processed by Alice and **passes**.
7. **Reissue the initial unroutable message.** The **same message ID** is now processed by Alice and **passes**.

**Expected:**

* Deterministic worker outcomes (pass/fail hooks) are honored.
* Reissued messages retain their original IDs and succeed after reissue.
* Unroutable message is recoverable once a valid consumer exists.
* End state: no pending failed/unroutable messages for this test.

---

## 3) Orphaned Skill Queues

**Goal:**
Show when a skills queue is active, when it becomes orphaned, and how pending messages behave.

**Steps (short & descriptive):**

1. **Start clean.** No characters exist.
   → The skills queue exists but is empty.
2. **Create a character “Alice.”**
   → Messages sent to the skills queue are processed by Alice.
3. **Delete Alice.**
   → The skills queue is still present, but now has no active character to consume from it — it is orphaned.
4. **Send some messages after Alice is gone.**
   → They stay pending in the orphaned queue.
5. **Create a new character “Bob.”**
   → The same skills queue becomes active again, and pending messages are consumed by Bob.

**Expected:**

* A skills queue can remain after a character is deleted.
* Orphaned queues accumulate pending messages.
* New characters re-activate the same queue and drain it.

