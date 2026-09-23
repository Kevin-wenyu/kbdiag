#!/usr/bin/env python3
"""Triage review findings with Jev (TypeSafe), see CLAUDE.md "OMC + Jev".

Reads {"items": [{"id", "claim", "evidence"}]} on stdin, where evidence is the
code or text the claim is about, and asks Jev in one request whether the
evidence supports each claim and how severe it is. The output is a triage
signal, not a verdict: every item is still checked by hand.

    TYPESAFE_API_KEY=... scripts/jev_review.py < items.json
"""
import json
import os
import sys
import urllib.request

URL = "https://api.typesafe.ai/v1/systemone"

VALIDITY = {
    "valid": "The evidence shows the claimed defect exists as described.",
    "invalid": "The evidence shows the claimed defect does not exist, or the code already handles it.",
    "needs_check": "The evidence is not enough to tell; it depends on code or behavior not shown.",
}
SEVERITY = [
    "Cosmetic: wording or style, no effect on what is tested or shipped.",
    "Low: a test could be stronger, but a real defect would still be caught elsewhere.",
    "Medium: a real defect could pass the tests unnoticed.",
    "High: shipped behavior is wrong, unsafe, or a test cannot fail at all.",
]


def main():
    items = json.load(sys.stdin)["items"]
    questions = {}
    for it in items:
        ref = f"`items.{it['id']}`"
        questions[it["id"] + ".validity"] = {
            "type": "choice",
            "instructions": f"A reviewer made the claim in {ref}.claim about the code or text in {ref}.evidence. "
                            "Judging only from the evidence, does the claimed defect exist?",
            "criteria": VALIDITY,
        }
        questions[it["id"] + ".severity"] = {
            "type": "score",
            "instructions": f"Assuming the claim in {ref}.claim is true, how severe is the defect it describes?",
            "criteria": SEVERITY,
        }
    body = {
        "model": "jev-latest",
        "state": {"items": {it["id"]: {"claim": it["claim"], "evidence": it["evidence"]} for it in items}},
        "questions": questions,
    }
    req = urllib.request.Request(URL, json.dumps(body).encode(), {
        "Authorization": "Bearer " + os.environ["TYPESAFE_API_KEY"],
        "Content-Type": "application/json",
    })
    with urllib.request.urlopen(req, timeout=60) as resp:
        out = json.load(resp)
    answers = out["answers"]
    for it in items:
        v, s = answers[it["id"] + ".validity"], answers[it["id"] + ".severity"]
        print(json.dumps({"id": it["id"], "validity": v, "severity": s}, ensure_ascii=False))
    print(json.dumps({"model": out["model"], "usage": out["usage"]}), file=sys.stderr)


if __name__ == "__main__":
    main()
