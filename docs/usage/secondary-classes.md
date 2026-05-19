# Secondary DNS Classes

## Overview

The secondary DNS classes feature allows the DNS controller manager to manage `DNSProviders` and `DNSEntries` from multiple DNS classes simultaneously.
Resources with secondary classes are automatically migrated to the primary class during reconciliation, enabling smooth transitions between DNS class configurations.

## Use Cases

### 1. Rolling Updates Between DNS Classes

When migrating from one DNS class to another across a large infrastructure:

**Before secondary classes:**
- Switching DNS class required coordination across multiple clusters
- Risk of DNS outage during the transition
- Manual cleanup of old class resources

**With secondary classes:**
- Deploy new DNS controller with secondary class configuration
- Existing resources continue to be managed
- Resources are automatically migrated to the new primary class
- No DNS service interruption

### 2. Multi-Tenant Environments

In shared environments where different teams use different DNS classes:

- A single DNS controller instance can manage resources from multiple classes
- Gradual consolidation of DNS classes without disruption
- Simplified operations by reducing the number of controller instances

### 3. Blue-Green Deployments

During DNS controller upgrades or configuration changes:

- Deploy new controller with different primary class
- Configure old class as secondary
- Resources seamlessly transition
- Rollback capability by reversing primary/secondary configuration

## Configuration

### Controller Configuration

Add secondary classes to the DNS manager configuration:

```yaml
apiVersion: dnsman2.gardener.cloud/v1alpha1
kind: DNSManagerConfiguration
class: "new-class"
secondaryClasses:
  - "old-class"
  - "legacy-class"
```

### Validation

The controller validates the configuration at startup:

- **No duplicates**: Secondary classes cannot contain duplicates (including normalized equivalents)
- **No conflicts**: Secondary classes cannot be equivalent to the primary class
- **Class normalization**: Empty string `""` and `"gardendns"` are treated as equivalent (default class)

## Migration Behavior

### Automatic Migration

When a DNSProvider or DNSEntry with a secondary class is reconciled:

1. **Class annotation update**:
   - If primary class is default (`gardendns`): annotation is removed
   - Otherwise: annotation is set to the primary class value

2. **Finalizer migration**:
   - Secondary class finalizers are removed (e.g., `old-class.dns.gardener.cloud/compound`)
   - Primary class finalizer is added (e.g., `new-class.dns.gardener.cloud/compound`)

3. **Secret handling** (DNSProvider only):
   - Associated secrets have their finalizers migrated as well
   - Ensures proper cleanup lifecycle

### Migration Example

**Before migration:**
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: my-entry
  annotations:
    dns.gardener.cloud/class: "old-class"
  finalizers:
    - old-class.dns.gardener.cloud/compound
spec:
  dnsName: "example.com"
```

**After migration** (with `class: "new-class"`, `secondaryClasses: ["old-class"]`):
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: my-entry
  annotations:
    dns.gardener.cloud/class: "new-class"
  finalizers:
    - new-class.dns.gardener.cloud/compound
spec:
  dnsName: "example.com"
```

### Timing

Migration happens during the reconciliation loop:
- **Before** regular reconciliation logic
- Ensures clean state before processing
- No requeue required - continues with normal reconciliation

## Rollout Strategy

### Step-by-Step Migration Guide

**Phase 1: Preparation**

1. Identify current DNS class in use (check existing resources)
2. Choose new primary class name
3. Update DNS controller configuration with secondary classes
4. Validate configuration locally

**Phase 2: Deployment**

1. Deploy updated DNS controller with secondary classes configured:
   ```yaml
   class: "new-primary"
   secondaryClasses:
     - "old-primary"
   ```

2. Controller will automatically start managing resources from both classes

**Phase 3: Migration**

3. Monitor logs for migration activity
4. Verify resources are being updated:
   ```bash
   kubectl get dnsentries -A -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.dns\.gardener\.cloud/class}{"\n"}{end}'
   ```

5. Check finalizers are migrated:
   ```bash
   kubectl get dnsproviders -A -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.finalizers}{"\n"}{end}'
   ```

**Phase 4: Cleanup**

6. Once all resources are migrated, remove secondary classes from configuration
7. Deploy final configuration with only the new primary class

**Phase 5: Verification**

8. Confirm no resources remain with old class annotation
9. Verify DNS resolution continues to work
10. Monitor for any issues in the first 24 hours


## Related Documentation

- [Controller Configuration Reference](../api-reference/config.md)
