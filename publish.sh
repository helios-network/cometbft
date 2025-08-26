VERSION=${VERSION:-"v0.50.10-helios-20"}

echo "Deploy cometbft"
git add .
git commit -m "Publish $VERSION"
git push
git tag $VERSION
git push origin $VERSION
GOPROXY=proxy.golang.org go list -m github.com/Helios-Chain-Labs/cometbft@$VERSION

echo "Publish done"