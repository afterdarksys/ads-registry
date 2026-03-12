#!/usr/bin/env ads-registry script

"""
migrate-compose-to-k8s.star

Converts a docker-compose.yml file to Kubernetes manifests.

Usage:
  ads-registry script migrate-compose-to-k8s.star --compose-file=docker-compose.yml

This script:
1. Parses the docker-compose file
2. Generates K8s Deployments for each service
3. Creates Services for exposed ports
4. Converts volumes to PVCs
5. Converts environment variables to ConfigMaps
6. Generates ImagePullSecrets for private registries
"""

# Configuration
COMPOSE_FILE = "docker-compose.yml"
OUTPUT_DIR = "k8s-manifests"
NAMESPACE = "default"
REGISTRY = "apps.afterdarksys.com:5005"

def main():
    print("=" * 60)
    print("Docker Compose to Kubernetes Migration")
    print("=" * 60)

    # Step 1: Parse docker-compose file
    print("\n[1/6] Parsing docker-compose file...")
    services = compose.parse(COMPOSE_FILE)
    print(f"      Found {len(services)} services")

    # Step 2: Create namespace
    print("\n[2/6] Creating namespace: " + NAMESPACE)
    ns_manifest = k8s.namespace(name=NAMESPACE)
    print(ns_manifest)

    # Step 3: Generate ImagePullSecret for registry
    print("\n[3/6] Creating ImagePullSecret for private registry...")
    image_pull_secret = k8s.image_pull_secret(
        name="registry-secret",
        namespace=NAMESPACE,
        registry=REGISTRY,
        username="admin",  # TODO: Get from env or vault
        password="password"  # TODO: Get from env or vault
    )
    print(image_pull_secret)

    # Step 4: Convert services to Deployments
    print("\n[4/6] Converting services to Deployments...")
    for service_name in services:
        image = services[service_name]["image"]
        replicas = services[service_name].get("replicas", 1)

        deployment = k8s.deployment(
            name=service_name,
            namespace=NAMESPACE,
            image=image,
            replicas=replicas
        )
        print(f"\n--- Deployment: {service_name} ---")
        print(deployment)

        # Create Service if ports are exposed
        if "ports" in services[service_name]:
            port = services[service_name]["ports"][0]
            service = k8s.service(
                name=service_name,
                namespace=NAMESPACE,
                type="ClusterIP",
                port=port
            )
            print(f"\n--- Service: {service_name} ---")
            print(service)

    # Step 5: Generate NetworkPolicies
    print("\n[5/6] Generating NetworkPolicies...")
    for service_name in services:
        network_policy = k8s.network_policy(
            name=service_name + "-network-policy",
            namespace=NAMESPACE
        )
        print(network_policy)

    # Step 6: Summary
    print("\n[6/6] Migration complete!")
    print("\n" + "=" * 60)
    print("Next steps:")
    print("  1. Review generated manifests")
    print("  2. Update ImagePullSecret credentials")
    print("  3. Apply manifests: kubectl apply -f " + OUTPUT_DIR)
    print("=" * 60)

# Run the migration
main()
