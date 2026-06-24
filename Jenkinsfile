pipeline {
    agent {
        docker {
            image 'casjaysdev/go:latest'
            args '-v go-state:/usr/local/share/go'
        }
    }

    options {
        disableConcurrentBuilds()
        timeout(time: 30, unit: 'MINUTES')
    }

    environment {
        CGO_ENABLED = '0'
        GOFLAGS = '-buildvcs=false'
    }

    stages {
        stage('Modules') {
            steps {
                sh 'go mod download'
            }
        }

        stage('Test') {
            steps {
                sh '''
                    PKGS=$(go list ./... | grep -v '/src/service')
                    mkdir -p /tmp/apimgr
                    COVDIR=$(mktemp -d /tmp/apimgr/search-XXXXXX)
                    go test -v -cover -coverprofile="$COVDIR/coverage.out" $PKGS
                    go tool cover -func="$COVDIR/coverage.out"
                    COVERAGE=$(go tool cover -func="$COVDIR/coverage.out" | \
                        awk '/^total:/{gsub(/%/,""); print $3}')
                    echo "Total coverage: ${COVERAGE}%"
                    awk "BEGIN{if (${COVERAGE}+0 < 80){print \"ERROR: Coverage ${COVERAGE}% < 80%\"; exit 1}}"
                '''
            }
        }

        stage('Lint') {
            steps {
                sh 'golangci-lint run ./...'
            }
        }

        stage('Build') {
            steps {
                sh '''
                    VERSION=$(cat release.txt 2>/dev/null || echo "devel")
                    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
                    BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
                    OFFICIALSITE=$(cat site.txt 2>/dev/null || echo "")
                    LDFLAGS="-s -w \
                        -X 'github.com/apimgr/search/src/config.Version=${VERSION}' \
                        -X 'github.com/apimgr/search/src/config.CommitID=${COMMIT}' \
                        -X 'github.com/apimgr/search/src/config.BuildDate=${BUILD_DATE}' \
                        -X 'github.com/apimgr/search/src/config.OfficialSite=${OFFICIALSITE}' \
                        -X 'github.com/apimgr/search/src/version.Version=${VERSION}' \
                        -X 'github.com/apimgr/search/src/version.Commit=${COMMIT}' \
                        -X 'github.com/apimgr/search/src/version.BuildDate=${BUILD_DATE}'"
                    GOOS=linux GOARCH=amd64 go build -ldflags="$LDFLAGS" -o binaries/search-linux-amd64 ./src
                    GOOS=linux GOARCH=arm64 go build -ldflags="$LDFLAGS" -o binaries/search-linux-arm64 ./src
                    echo "Build OK"
                '''
            }
        }

        stage('Security') {
            parallel {
                stage('Vulnerability scan') {
                    steps {
                        sh 'govulncheck ./...'
                    }
                }
                stage('Secret scan') {
                    steps {
                        sh '''
                            docker run --rm \
                                --name "trufflehog-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" \
                                -v "$(pwd):/repo" \
                                trufflesecurity/trufflehog:latest \
                                git file:///repo --since-commit HEAD~1 --only-verified --fail
                        '''
                    }
                }
            }
        }

        stage('Release') {
            when {
                tag pattern: '^(v[0-9]|[0-9]+\\.[0-9]+\\.[0-9]+)', comparator: 'REGEXP'
            }
            steps {
                sh '''
                    VERSION=$(cat release.txt 2>/dev/null || echo "${TAG_NAME}")
                    echo "Release: ${VERSION}"
                '''
            }
        }
    }

    post {
        always {
            cleanWs()
        }
    }
}
