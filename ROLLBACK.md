# Branch Creation Rollback Mechanism

The PR Splitter CLI includes a comprehensive rollback mechanism to ensure your git repository remains in a clean state if anything goes wrong during the branch creation process.

## 🔄 **Automatic Rollback**

### When Rollback Triggers

The automatic rollback mechanism activates when:

1. **Branch Creation Failure**: If creating any branch fails
2. **File Operation Failure**: If applying changes to a branch fails  
3. **Commit Failure**: If committing changes fails
4. **Push Failure**: If pushing a branch to remote fails
5. **Validation Failure**: If pre/post validation checks fail
6. **Panic/Crash**: If the application crashes unexpectedly

### What Gets Rolled Back

When rollback triggers, the system:

1. **Stops Processing**: Immediately halts creation of additional branches
2. **Returns to Original Branch**: Checks out the branch you were on before starting
3. **Deletes Remote Branches**: Removes any branches that were successfully pushed
4. **Deletes Local Branches**: Removes any branches that were created locally
5. **Preserves Repository State**: Ensures no partial changes remain

### Rollback Safety Features

- **Original Branch Preservation**: Your starting branch is never modified
- **Working Directory Safety**: Uncommitted changes are preserved  
- **Dependency Order**: Branches are deleted in reverse dependency order
- **Error Tolerance**: Continues rollback even if some deletions fail
- **Detailed Logging**: Shows exactly what was cleaned up

## 📋 **Manual Rollback Command**

For cases where you need to manually clean up branches (e.g., after a partial success), use the rollback command:

```bash
pr-split rollback [branch-prefix]
```

### Examples

```bash
# Clean up all branches starting with 'pr-split'
pr-split rollback pr-split

# Clean up branches with custom prefix
pr-split rollback feature-auth-split

# Clean up specific ticket branches
pr-split rollback WIS-4721
```

### Manual Rollback Process

1. **Branch Discovery**: Finds all local and remote branches matching the prefix
2. **Preview**: Shows exactly what will be deleted before proceeding
3. **Confirmation**: Asks for explicit user confirmation
4. **Safe Checkout**: Moves to a safe branch (main/master) if current branch will be deleted
5. **Remote Cleanup**: Deletes remote branches first
6. **Local Cleanup**: Deletes local branches
7. **Status Report**: Shows final state and any warnings

### Sample Output

```
🔍 Searching for branches with prefix: pr-split

📋 Found branches to delete:

Local branches (3):
  🔸 pr-split-1-auth-components
  🔸 pr-split-2-auth-utils  
  🔸 pr-split-3-auth-tests

Remote branches (3):
  🔸 pr-split-1-auth-components
  🔸 pr-split-2-auth-utils
  🔸 pr-split-3-auth-tests

Delete 3 local and 3 remote branches? [y/N]: y

🔄 Starting rollback...
💼 Checked out to safe branch: main
🗑️  Deleting remote branch: pr-split-1-auth-components
✅ Deleted remote branch: pr-split-1-auth-components
...
🎉 Rollback completed successfully!
📍 Currently on branch: main
```

## 🛡️ **Safety Mechanisms**

### Pre-Creation Validation

Before creating any branches, the system validates:

- **Repository State**: Working directory is clean
- **Branch Existence**: No conflicting branch names exist
- **Permissions**: Can create and push branches
- **Dependencies**: All dependency branches are available

### During Creation Tracking

The system maintains detailed state:

```go
var createdBranches []string    // Branches created locally
var pushedBranches []string     // Branches pushed to remote  
var originalBranch string       // Starting branch for return
```

### Rollback Error Handling

Even if rollback encounters errors:

- **Continues Processing**: Tries to clean up as much as possible
- **Logs Warnings**: Shows what couldn't be deleted
- **Preserves State**: Ensures repository is still functional
- **Manual Recovery**: Provides commands for manual cleanup

## 🔧 **Advanced Usage**

### Dry Run Mode (Future Enhancement)

```bash
# Preview what would be rolled back without deleting
pr-split rollback --dry-run pr-split
```

### Force Rollback (Future Enhancement)  

```bash
# Skip confirmation prompts
pr-split rollback --force pr-split
```

### Rollback with Filters (Future Enhancement)

```bash
# Only rollback branches newer than 1 day
pr-split rollback --max-age=1d pr-split

# Only rollback local branches
pr-split rollback --local-only pr-split
```

## 🚨 **Emergency Recovery**

If automatic rollback fails and you need to manually clean up:

### Manual Git Commands

```bash
# List branches matching pattern
git branch | grep "pr-split"
git branch -r | grep "pr-split"

# Delete local branches
git branch -D pr-split-1-auth-components
git branch -D pr-split-2-auth-utils

# Delete remote branches  
git push origin --delete pr-split-1-auth-components
git push origin --delete pr-split-2-auth-utils

# Return to safe branch
git checkout main
```

### Verification Commands

```bash
# Verify cleanup completed
git branch | grep "pr-split"           # Should return nothing
git branch -r | grep "pr-split"        # Should return nothing

# Check repository status
git status                             # Should be clean
git log --oneline -5                   # Should show expected history
```

## 📝 **Best Practices**

### Before Running PR Split

1. **Commit Changes**: Ensure working directory is clean
2. **Backup Important Work**: Create backup branches if needed
3. **Test Connection**: Verify you can push to remote
4. **Check Naming**: Ensure branch prefix won't conflict

### After Rollback

1. **Verify State**: Check that repository is clean
2. **Review Logs**: Understand what caused the rollback
3. **Fix Issues**: Address underlying problems before retrying  
4. **Test Manually**: Consider creating one test branch first

### Rollback Monitoring

The system provides detailed logging for monitoring:

```
🔄 Rolling back branch creation...
💼 Checked out to original branch: feature/large-pr
🗑️  Deleting remote branch: pr-split-1-components
✅ Deleted remote branch: pr-split-1-components
🗑️  Deleting local branch: pr-split-1-components  
✅ Deleted local branch: pr-split-1-components
🔄 Rollback completed. Repository returned to clean state.
```

## 🔍 **Troubleshooting**

### Common Issues

**"Could not delete remote branch"**
- Check network connectivity
- Verify push permissions to remote
- Branch might already be deleted

**"Could not checkout original branch"**  
- Original branch might have been deleted
- System will fallback to main/master
- Check branch name spelling

**"Working directory has uncommitted changes"**
- Commit or stash changes before running
- Use `git status` to see what's uncommitted
- Rollback preserves these changes

### Debug Mode

Enable verbose logging:

```bash
# Set debug mode for detailed output
export PR_SPLIT_DEBUG=1
pr-split break feature/large-branch
```

This comprehensive rollback mechanism ensures that the PR Splitter CLI can safely recover from any failure scenario, keeping your repository in a clean and functional state. 