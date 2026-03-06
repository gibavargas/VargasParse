# GitHub Publish (GPL v2)

## 1) Initialize and Commit

```bash
git init
git add .
git commit -m "docs: add extensive documentation and GPL v2 license"
```

## 2) Create GitHub Repository

Using GitHub CLI:

```bash
gh auth login
gh repo create VargasParse --public --source=. --remote=origin --push
```

Or manually:

```bash
git remote add origin git@github.com:<your-user>/VargasParse.git
git branch -M main
git push -u origin main
```

## 3) Verify License on GitHub

- Ensure `LICENSE` is at repository root.
- Ensure README says GPL v2.

## 4) Optional Release Tag

```bash
git tag -a v0.1.0 -m "Initial GPLv2 documented release"
git push origin v0.1.0
```
