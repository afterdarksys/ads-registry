#!/usr/bin/env ads-registry script

"""
supply-chain-report.star

Generates a comprehensive supply chain security report for all images owned by a user.

Usage:
  ads-registry script supply-chain-report.star --user=ryan

This script:
1. Lists all images owned by the user
2. Analyzes supply chain security posture (SBOM, provenance, signatures)
3. Generates gap analysis report
4. Provides recommendations for improvement
"""

# Configuration
USER_ID = 1  # TODO: Get from args
OUTPUT_FILE = "supply-chain-report.html"
MIN_ACCEPTABLE_SCORE = 70  # Minimum security score to pass

def generate_html_report(reports):
    """Generate HTML report from analysis results"""
    html = """
    <!DOCTYPE html>
    <html>
    <head>
        <title>Supply Chain Security Report</title>
        <style>
            body { font-family: Arial, sans-serif; margin: 40px; }
            h1 { color: #333; }
            .summary { background: #f0f0f0; padding: 20px; border-radius: 5px; }
            .image { margin: 20px 0; padding: 15px; border: 1px solid #ddd; }
            .score { font-size: 24px; font-weight: bold; }
            .good { color: green; }
            .warning { color: orange; }
            .critical { color: red; }
            .gap { background: #fff3cd; padding: 10px; margin: 5px 0; border-left: 4px solid #ffc107; }
        </style>
    </head>
    <body>
        <h1>Supply Chain Security Report</h1>
    """

    # Summary statistics
    total_images = len(reports)
    total_score = sum(r["overall_score"] for r in reports)
    avg_score = total_score / total_images if total_images > 0 else 0
    passing = sum(1 for r in reports if r["overall_score"] >= MIN_ACCEPTABLE_SCORE)

    html += f"""
        <div class="summary">
            <h2>Summary</h2>
            <p><strong>Total Images:</strong> {total_images}</p>
            <p><strong>Average Security Score:</strong> <span class="score">{avg_score:.1f}/100</span></p>
            <p><strong>Passing Images:</strong> {passing}/{total_images} ({100*passing/total_images if total_images > 0 else 0:.1f}%)</p>
        </div>
    """

    # Individual image reports
    html += "<h2>Individual Image Analysis</h2>"

    for report in reports:
        score_class = "good" if report["overall_score"] >= 80 else "warning" if report["overall_score"] >= 60 else "critical"

        html += f"""
        <div class="image">
            <h3>{report["image_name"]}</h3>
            <p><strong>Digest:</strong> {report["image_digest"]}</p>
            <p><strong>Security Score:</strong> <span class="score {score_class}">{report["overall_score"]}/100</span></p>
            <p><strong>Maturity Level:</strong> {report["maturity_level"]}</p>

            <h4>Components</h4>
            <ul>
                <li>SBOM: {"✓ Present" if report["sbom"]["present"] else "✗ Missing"}</li>
                <li>Provenance: {"✓ Present (SLSA L" + str(report["provenance"]["slsa_level"]) + ")" if report["provenance"]["present"] else "✗ Missing"}</li>
                <li>Signatures: {"✓ Signed" if report["signatures"]["signed"] else "✗ Not signed"}</li>
                <li>Dependencies: {report["dependencies"]["total_dependencies"]} total</li>
            </ul>
        """

        # Show gaps
        if report["gaps"]:
            html += "<h4>Security Gaps</h4>"
            for gap in report["gaps"]:
                html += f"""
                <div class="gap">
                    <strong>[{gap["severity"].upper()}] {gap["description"]}</strong><br>
                    <em>Impact:</em> {gap["impact"]}<br>
                    <em>Remediation:</em> {gap["remediation"]}
                </div>
                """

        # Show recommendations
        if report["recommendations"]:
            html += "<h4>Recommendations</h4><ul>"
            for rec in report["recommendations"]:
                html += f"<li>{rec}</li>"
            html += "</ul>"

        html += "</div>"

    html += """
    </body>
    </html>
    """

    return html

def main():
    print("=" * 60)
    print("Supply Chain Security Report Generator")
    print("=" * 60)

    # Get all images for the user
    print("\n[1/4] Fetching user images...")
    images = registry.list_images(owner_id=USER_ID)
    print(f"      Found {len(images)} images")

    # Analyze each image
    print("\n[2/4] Analyzing supply chain security...")
    reports = []

    for i, image in enumerate(images):
        print(f"\n      [{i+1}/{len(images)}] Analyzing {image['name']}...")

        # Run supply chain analysis
        analysis = scan.analyze_supply_chain(image["digest"])

        # Extract key metrics
        report = {
            "image_name": image["name"],
            "image_digest": image["digest"],
            "overall_score": analysis["overall_score"],
            "maturity_level": analysis["maturity_level"],
            "sbom": analysis["sbom"],
            "provenance": analysis["provenance"],
            "signatures": analysis["signatures"],
            "dependencies": analysis["dependencies"],
            "gaps": analysis["gaps"],
            "recommendations": analysis["recommendations"]
        }

        reports.append(report)

        # Show quick summary
        print(f"            Score: {report['overall_score']}/100 ({report['maturity_level']})")
        print(f"            SBOM: {'✓' if report['sbom']['present'] else '✗'}")
        print(f"            Provenance: {'✓ (L' + str(report['provenance']['slsa_level']) + ')' if report['provenance']['present'] else '✗'}")
        print(f"            Signed: {'✓' if report['signatures']['signed'] else '✗'}")
        print(f"            Gaps: {len(report['gaps'])}")

    # Generate statistics
    print("\n[3/4] Generating statistics...")
    total_gaps = sum(len(r["gaps"]) for r in reports)
    critical_gaps = sum(1 for r in reports for g in r["gaps"] if g["severity"] == "critical")
    unsigned_images = sum(1 for r in reports if not r["signatures"]["signed"])
    no_sbom = sum(1 for r in reports if not r["sbom"]["present"])
    no_provenance = sum(1 for r in reports if not r["provenance"]["present"])

    print(f"\n      Total security gaps: {total_gaps}")
    print(f"      Critical gaps: {critical_gaps}")
    print(f"      Unsigned images: {unsigned_images}/{len(images)}")
    print(f"      Missing SBOM: {no_sbom}/{len(images)}")
    print(f"      Missing provenance: {no_provenance}/{len(images)}")

    # Generate HTML report
    print("\n[4/4] Generating HTML report...")
    html = generate_html_report(reports)

    # Save report
    with open(OUTPUT_FILE, "w") as f:
        f.write(html)

    print(f"\n      ✓ Report saved to {OUTPUT_FILE}")

    print("\n" + "=" * 60)
    print("Report Generation Complete!")
    print("=" * 60)

    # Final recommendations
    if critical_gaps > 0:
        print(f"\n⚠️  WARNING: {critical_gaps} critical security gaps found!")
        print("   Immediate action required to improve supply chain security.")

    if unsigned_images > 0:
        print(f"\n   Recommendation: Sign {unsigned_images} images using Cosign")

    if no_sbom > 0:
        print(f"\n   Recommendation: Generate SBOMs for {no_sbom} images")

    if no_provenance > 0:
        print(f"\n   Recommendation: Add provenance attestations to {no_provenance} images")

main()
