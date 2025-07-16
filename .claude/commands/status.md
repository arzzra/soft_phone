# 📊 Session Status Report

Generate a comprehensive status report for the current development session.

## Current Task Overview

Analyze and report:
- **Active Feature/Task**: What is currently being worked on
- **Current Phase**: Which phase of the workflow are we in
- **Overall Progress**: Percentage complete with visual progress bar
- **Time Elapsed**: How long has this task been active

## 🤖 Sub-Agent Activity

List all sub-agents and their current state:

### Active Agents
- [AGENT: NAME] - Current task/status
- Show which agents are currently engaged
- What each agent is working on
- Any pending agent tasks

### Available Agents
- List agents not yet engaged
- Upcoming agent activities

## ✅ Task Checklist

Display comprehensive task list:
- [ ] Exploration complete
- [ ] Plan approved by user
- [ ] Tests written (RED phase)
- [ ] All tests failing appropriately
- [ ] Implementation complete (GREEN phase)
- [ ] All tests passing
- [ ] Refactoring complete
- [ ] Security audit passed
- [ ] Performance optimization done
- [ ] Documentation updated
- [ ] Code review complete
- [ ] Ready for commit

## 📈 Metrics Dashboard

### Test Metrics
- **Tests Written**: [count]
- **Tests Passing**: [count]/[total]
- **Test Coverage**: [percentage]%
- **Test Execution Time**: [time]

### Code Metrics
- **Files Modified**: [count]
- **Lines Added**: [count]
- **Lines Removed**: [count]
- **Complexity Score**: [if available]

### Quality Metrics
- **Code Smells Found**: [count]
- **Security Issues**: [count]
- **Performance Issues**: [count]

## 🚧 Current Blockers

List any blocking issues:
- Technical blockers
- Failing tests details
- Missing requirements
- Pending decisions

## 🔄 Next Steps

Ordered list of upcoming actions:
1. Next immediate action
2. Following steps
3. Estimated time to completion

## 💡 Quick Actions

Available commands:
- `/continue` - Proceed with next step
- `/tests` - Run test suite
- `/review` - Trigger code review
- `/abort` - Cancel current task

## Session Configuration

Display current session settings:
- **TDD Mode**: [Strict/Standard]
- **Coverage Target**: [percentage]%
- **Security Audit**: [Enabled/Disabled]
- **Performance Checks**: [Enabled/Disabled]

## Recent Activity Log

Show last 5 significant actions:
- [timestamp] Action performed
- [timestamp] Tests run - X/Y passing
- [timestamp] File modified
- etc.

---

If no active session exists, provide guidance:
- How to start a new task
- Available commands
- Quick start suggestions