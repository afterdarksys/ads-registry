# sync_policy.star
# This Starlark script controls which images are allowed to be synchronized
# to peer registries (e.g. for high availability or geographic distribution).
# 
# The function `on_sync_attempt(event)` is called by the Go registry engine
# before starting a transfer. If it returns False, the sync is aborted.

def on_sync_attempt(event):
    print("Evaluating Sync Policy for: " + event.data["repository"])
    
    # Example: Block synchronization to the 'registry-gov' peer if the image
    # is not pushed to the 'approved/' namespace.
    if event.data["peer_name"] == "registry-gov":
        if not event.data["repository"].startswith("approved/"):
            print("Sync Policy Violation: Only 'approved/' images can go to Gov Cloud")
            return False
            
    # Example: Put a specific region offline temporarily
    if event.data["peer_name"] == "registry-eu-west":
        # Offline for maintenance
        return False
        
    return True
