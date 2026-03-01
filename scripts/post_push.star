def on_push(event):
    print("Detected push for digest: " + event.data["digest"])
    if event.data["namespace"] == "production":
        print("Production image updated, triggering GitOps pipeline...")
        # Since this is a test, we will trigger a webhook to a generic endpoint like httpbin or our own server
        resp = http_post("http://localhost:5005/", "{\"event\":\"image_updated\"}")
        print("Webhook response code: ", resp[0])
    
    return True
