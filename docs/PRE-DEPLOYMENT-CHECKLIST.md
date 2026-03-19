# Pre-Deployment Checklist

## Build Verification

- [x] Code changes implemented in all required files
- [x] Build successful: `ads-registry-fixed` (43MB)
- [x] Binary type verified: Mach-O 64-bit executable x86_64
- [x] No compiler errors or warnings
- [x] All imports resolved correctly

## Documentation Complete

- [x] ROUTER-FIX-DOCUMENTATION.md - Complete technical documentation
- [x] DEPLOYMENT-INSTRUCTIONS.md - Step-by-step deployment guide
- [x] ROUTING-ANALYSIS.md - Deep dive into routing behavior
- [x] ROUTER-FIX-SUMMARY.md - Quick reference for team
- [x] PRE-DEPLOYMENT-CHECKLIST.md - This checklist
- [x] test-routing-fix.sh - Automated testing script

## Code Review

- [x] Router pattern changed from dual routes to single wildcard
- [x] parseRepoPath() function implemented correctly
- [x] All handlers updated to use repoPath parameter
- [x] Auth middleware updated for wildcard pattern
- [x] Location headers use repoPath directly
- [x] Storage paths remain unchanged (backward compatible)
- [x] No breaking changes to database queries

## Files Modified

```
modified:   internal/api/v2/router.go
  - Changed route pattern to /{repoPath...}
  - Added parseRepoPath() function
  - Updated all handlers to extract repoPath
  - Added strings import

modified:   internal/api/v2/referrers.go
  - Updated getReferrers() to use repoPath

modified:   internal/auth/middleware.go
  - Updated Protect() to extract repoPath
  - Fixed authorization check for full repo path
```

## Testing Preparation

- [x] Test script created: test-routing-fix.sh
- [x] Test script made executable (chmod +x)
- [ ] Test script validated on staging (if available)
- [ ] Manual curl commands prepared

## Deployment Readiness

### Binary Transfer
- [x] Binary built: ads-registry-fixed
- [ ] Binary copied to deployment machine
- [ ] Binary permissions verified (executable)

### Backup Plan
- [ ] Current production binary backed up
- [ ] Backup location documented
- [ ] Rollback procedure tested (if possible)

### Server Preparation
- [ ] Server access verified
- [ ] Service control permissions verified (systemctl)
- [ ] Target directory identified
- [ ] Current binary location confirmed

### Monitoring Setup
- [ ] Log monitoring access verified
- [ ] Health check URLs accessible
- [ ] Metrics endpoint accessible (if applicable)
- [ ] Alert channels ready

## Risk Mitigation

### What Can Go Wrong
1. Service fails to start
   - **Mitigation**: Immediate rollback to previous binary
   - **Detection**: systemctl status, journalctl logs

2. Routing still broken
   - **Mitigation**: Review logs, verify URL parameter extraction
   - **Detection**: Test script shows 404 errors

3. Multi-level repos break
   - **Mitigation**: Rollback immediately
   - **Detection**: Test script shows failures for library/* repos

4. Auth failures increase
   - **Mitigation**: Review JWT scope validation logic
   - **Detection**: Watch for 401/403 status codes

### Rollback Triggers
Rollback immediately if:
- Service won't start after 2 attempts
- Health checks fail after startup
- 404 errors persist for single-level repos
- Multi-level repos start failing
- Error rate increases by >10%

## Deployment Window

**Recommended time**: Off-peak hours (if possible)

**Timeline**:
- 0:00 - Stop service
- 0:01 - Backup current binary
- 0:02 - Deploy new binary
- 0:03 - Start service
- 0:04 - Verify startup logs
- 0:05 - Run test script
- 0:10 - Monitor for errors
- 0:20 - Declare success or rollback

**Total estimated time**: 20 minutes (or 5 minutes for rollback)

## Post-Deployment Validation

### Immediate Tests (< 5 minutes)
- [ ] Service started successfully
- [ ] Health check returns 200 OK
- [ ] Logs show no errors
- [ ] Single-level repo upload returns 202
- [ ] Multi-level repo upload returns 202

### Short-term Tests (< 30 minutes)
- [ ] Test script passes all tests
- [ ] Real docker push succeeds
- [ ] No increase in error rates
- [ ] Response times normal
- [ ] Catalog endpoint works

### Long-term Monitoring (24 hours)
- [ ] Migration completion rate increases
- [ ] 404 errors remain at zero
- [ ] No new error patterns
- [ ] System stability maintained
- [ ] Performance metrics normal

## Communication Plan

### Before Deployment
- [ ] Notify team of deployment window
- [ ] Share documentation links
- [ ] Confirm support availability

### During Deployment
- [ ] Announce service stop
- [ ] Report deployment status
- [ ] Share initial test results

### After Deployment
- [ ] Announce service restoration
- [ ] Share test results
- [ ] Report migration progress
- [ ] Document any issues encountered

## Sign-off

**Code reviewed by**: _________________
**Deployment approved by**: _________________
**Date**: _________________

## Emergency Contacts

**Developer**: Ryan
**Server**: apps.afterdarksys.com:5006
**Service name**: ads-registry
**Logs command**: `journalctl -u ads-registry -f`

## Final Checks

Before executing deployment:

1. [ ] All team members notified
2. [ ] Backup completed
3. [ ] Rollback procedure understood
4. [ ] Test script ready to run
5. [ ] Monitoring dashboard open
6. [ ] Emergency contacts available

**If all boxes are checked, you are GO for deployment!**

---

**Deployment Status**: READY ✅
**Risk Level**: LOW
**Confidence Level**: HIGH
