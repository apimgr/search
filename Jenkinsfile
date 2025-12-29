pipeline {
    agent none

    triggers {
        // Daily build at 3am UTC (matches GitHub Actions daily.yml)
        cron('0 3 * * *')
    }

    environment {
        PROJECTNAME = 'search'
        PROJECTORG = 'apimgr'
        BINDIR = 'binaries'
        RELDIR = 'releases'
        GOCACHE = '/tmp/go-cache'
        GOMODCACHE = '/tmp/go-mod-cache'

        // =========================================================================
        // GIT PROVIDER CONFIGURATION
        // Uncomment ONE block below based on your git hosting platform
        // =========================================================================

        // ----- GITHUB (default) -----
        GIT_FQDN = 'github.com'
        GIT_TOKEN = credentials('github-token')  // Jenkins credentials ID
        REGISTRY = "ghcr.io/${PROJECTORG}/${PROJECTNAME}"

        // ----- GITEA / FORGEJO (self-hosted) -----
        // GIT_FQDN = 'git.example.com'  // Your Gitea/Forgejo domain
        // GIT_TOKEN = credentials('gitea-token')  // Jenkins credentials ID
        // REGISTRY = "${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}"

        // ----- GITLAB (gitlab.com or self-hosted) -----
        // GIT_FQDN = 'gitlab.com'  // or your self-hosted GitLab domain
        // GIT_TOKEN = credentials('gitlab-token')  // Jenkins credentials ID
        // REGISTRY = "registry.${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}"

        // ----- DOCKER HUB -----
        // GIT_FQDN = 'github.com'  // Git host (separate from registry)
        // GIT_TOKEN = credentials('github-token')
        // REGISTRY = "docker.io/${PROJECTORG}/${PROJECTNAME}"

        // =========================================================================
    }

    stages {
        stage('Setup') {
            agent { label 'amd64' }
            steps {
                script {
                    // Determine build type and version
                    if (env.TAG_NAME) {
                        // Release build (tag push) - matches release.yml
                        env.BUILD_TYPE = 'release'
                        env.VERSION = env.TAG_NAME.replaceFirst('^v', '')
                    } else if (env.BRANCH_NAME == 'beta') {
                        // Beta build - matches beta.yml
                        env.BUILD_TYPE = 'beta'
                        env.VERSION = sh(script: 'date -u +"%Y%m%d%H%M%S"', returnStdout: true).trim() + '-beta'
                    } else if (env.BRANCH_NAME == 'main' || env.BRANCH_NAME == 'master') {
                        // Daily build - matches daily.yml
                        env.BUILD_TYPE = 'daily'
                        env.VERSION = sh(script: 'date -u +"%Y%m%d%H%M%S"', returnStdout: true).trim()
                    } else {
                        // Other branches - dev build
                        env.BUILD_TYPE = 'dev'
                        env.VERSION = sh(script: 'date -u +"%Y%m%d%H%M%S"', returnStdout: true).trim() + '-dev'
                    }
                    env.COMMIT_ID = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
                    env.BUILD_DATE = sh(script: 'date +"%a %b %d, %Y at %H:%M:%S %Z"', returnStdout: true).trim()
                    env.LDFLAGS = "-s -w -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/config.Version=${env.VERSION}' -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/config.CommitID=${env.COMMIT_ID}' -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/config.BuildDate=${env.BUILD_DATE}'"
                    env.CLI_LDFLAGS = "-s -w -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/client/cmd.ProjectName=${PROJECTNAME}' -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/client/cmd.Version=${env.VERSION}' -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/client/cmd.CommitID=${env.COMMIT_ID}' -X 'github.com/${PROJECTORG}/${PROJECTNAME}/src/client/cmd.BuildDate=${env.BUILD_DATE}'"
                    env.HAS_CLI = sh(script: '[ -d src/client ] && echo true || echo false', returnStdout: true).trim()
                }
                sh 'mkdir -p ${BINDIR} ${RELDIR}'
                echo "Build type: ${BUILD_TYPE}, Version: ${VERSION}"
            }
        }

        stage('Build Server') {
            parallel {
                // Linux
                stage('Linux AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=linux \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-linux-amd64 ./src
                        '''
                    }
                }
                stage('Linux ARM64') {
                    agent { label 'arm64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=linux \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-linux-arm64 ./src
                        '''
                    }
                }
                // Darwin (macOS)
                stage('Darwin AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=darwin \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-darwin-amd64 ./src
                        '''
                    }
                }
                stage('Darwin ARM64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=darwin \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-darwin-arm64 ./src
                        '''
                    }
                }
                // Windows
                stage('Windows AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=windows \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-windows-amd64.exe ./src
                        '''
                    }
                }
                stage('Windows ARM64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=windows \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-windows-arm64.exe ./src
                        '''
                    }
                }
                // FreeBSD
                stage('FreeBSD AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=freebsd \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-freebsd-amd64 ./src
                        '''
                    }
                }
                stage('FreeBSD ARM64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=freebsd \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-freebsd-arm64 ./src
                        '''
                    }
                }
            }
        }

        // CLI builds - only if src/client/ exists (matches GitHub Actions)
        stage('Build CLI') {
            when {
                expression { env.HAS_CLI == 'true' }
            }
            parallel {
                stage('CLI Linux AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=linux \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-linux-amd64-cli ./src/client
                        '''
                    }
                }
                stage('CLI Linux ARM64') {
                    agent { label 'arm64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=linux \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-linux-arm64-cli ./src/client
                        '''
                    }
                }
                stage('CLI Darwin AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=darwin \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-darwin-amd64-cli ./src/client
                        '''
                    }
                }
                stage('CLI Darwin ARM64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=darwin \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-darwin-arm64-cli ./src/client
                        '''
                    }
                }
                stage('CLI Windows AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=windows \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-windows-amd64-cli.exe ./src/client
                        '''
                    }
                }
                stage('CLI Windows ARM64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=windows \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-windows-arm64-cli.exe ./src/client
                        '''
                    }
                }
                stage('CLI FreeBSD AMD64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=freebsd \
                                -e GOARCH=amd64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-freebsd-amd64-cli ./src/client
                        '''
                    }
                }
                stage('CLI FreeBSD ARM64') {
                    agent { label 'amd64' }
                    steps {
                        sh '''
                            docker run --rm \
                                -v ${WORKSPACE}:/build \
                                -v ${GOCACHE}:/root/.cache/go-build \
                                -v ${GOMODCACHE}:/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                -e GOOS=freebsd \
                                -e GOARCH=arm64 \
                                golang:alpine \
                                go build -ldflags "${CLI_LDFLAGS}" -o ${BINDIR}/${PROJECTNAME}-freebsd-arm64-cli ./src/client
                        '''
                    }
                }
            }
        }

        stage('Test') {
            agent { label 'amd64' }
            steps {
                sh '''
                    docker run --rm \
                        -v ${WORKSPACE}:/build \
                        -v ${GOCACHE}:/root/.cache/go-build \
                        -v ${GOMODCACHE}:/go/pkg/mod \
                        -w /build \
                        golang:alpine \
                        go test -v -cover ./...
                '''
            }
        }

        // Stable Release - matches release.yml (tag push only)
        stage('Release: Stable') {
            agent { label 'amd64' }
            when {
                expression { env.BUILD_TYPE == 'release' }
            }
            steps {
                sh '''
                    echo "${VERSION}" > ${RELDIR}/version.txt

                    for f in ${BINDIR}/${PROJECTNAME}-*; do
                        [ -f "$f" ] || continue
                        cp "$f" ${RELDIR}/
                    done

                    tar --exclude='.git' --exclude='.github' --exclude='.gitea' \
                        --exclude='.forgejo' --exclude='binaries' --exclude='releases' \
                        --exclude='*.tar.gz' \
                        -czf ${RELDIR}/${PROJECTNAME}-${VERSION}-source.tar.gz .
                '''
                archiveArtifacts artifacts: 'releases/*', fingerprint: true
            }
        }

        // Beta Release - matches beta.yml (beta branch only)
        stage('Release: Beta') {
            agent { label 'amd64' }
            when {
                expression { env.BUILD_TYPE == 'beta' }
            }
            steps {
                sh '''
                    echo "${VERSION}" > ${RELDIR}/version.txt

                    for f in ${BINDIR}/${PROJECTNAME}-*; do
                        [ -f "$f" ] || continue
                        cp "$f" ${RELDIR}/
                    done
                '''
                archiveArtifacts artifacts: 'releases/*', fingerprint: true
            }
        }

        // Daily Release - matches daily.yml (main/master + scheduled)
        stage('Release: Daily') {
            agent { label 'amd64' }
            when {
                expression { env.BUILD_TYPE == 'daily' }
            }
            steps {
                sh '''
                    echo "${VERSION}" > ${RELDIR}/version.txt

                    for f in ${BINDIR}/${PROJECTNAME}-*; do
                        [ -f "$f" ] || continue
                        cp "$f" ${RELDIR}/
                    done
                '''
                archiveArtifacts artifacts: 'releases/*', fingerprint: true
            }
        }

        // Docker - matches docker.yml (ALL branches and tags)
        stage('Docker') {
            agent { label 'amd64' }
            steps {
                script {
                    def tags = "-t ${REGISTRY}:${COMMIT_ID}"

                    if (env.BUILD_TYPE == 'release') {
                        // Release tag - version, latest, YYMM
                        def yymm = new Date().format('yyMM')
                        tags += " -t ${REGISTRY}:${VERSION}"
                        tags += " -t ${REGISTRY}:latest"
                        tags += " -t ${REGISTRY}:${yymm}"
                    } else if (env.BUILD_TYPE == 'beta') {
                        // Beta branch - beta, devel
                        tags += " -t ${REGISTRY}:beta"
                        tags += " -t ${REGISTRY}:devel"
                    } else {
                        // All other branches - devel only
                        tags += " -t ${REGISTRY}:devel"
                    }

                    // Login to container registry
                    // Works with: ghcr.io, registry.gitlab.com, gitea/forgejo, docker.io
                    sh """
                        echo "\${GIT_TOKEN}" | docker login ${REGISTRY.split('/')[0]} -u ${PROJECTORG} --password-stdin
                    """

                    // Build multi-arch with OCI labels and manifest annotations
                    sh """
                        docker buildx create --name ${PROJECTNAME}-builder --use 2>/dev/null || docker buildx use ${PROJECTNAME}-builder
                        docker buildx build \
                            -f docker/Dockerfile \
                            --platform linux/amd64,linux/arm64 \
                            --build-arg VERSION="${VERSION}" \
                            --build-arg COMMIT_ID="${COMMIT_ID}" \
                            --build-arg BUILD_DATE="${BUILD_DATE}" \
                            --label "org.opencontainers.image.vendor=${PROJECTORG}" \
                            --label "org.opencontainers.image.authors=${PROJECTORG}" \
                            --label "org.opencontainers.image.title=${PROJECTNAME}" \
                            --label "org.opencontainers.image.base.name=${PROJECTNAME}" \
                            --label "org.opencontainers.image.description=Containerized version of ${PROJECTNAME}" \
                            --label "org.opencontainers.image.licenses=MIT" \
                            --label "org.opencontainers.image.version=${VERSION}" \
                            --label "org.opencontainers.image.created=${BUILD_DATE}" \
                            --label "org.opencontainers.image.revision=${COMMIT_ID}" \
                            --label "org.opencontainers.image.url=https://${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}" \
                            --label "org.opencontainers.image.source=https://${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}" \
                            --label "org.opencontainers.image.documentation=https://${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}" \
                            --annotation "manifest:org.opencontainers.image.vendor=${PROJECTORG}" \
                            --annotation "manifest:org.opencontainers.image.authors=${PROJECTORG}" \
                            --annotation "manifest:org.opencontainers.image.title=${PROJECTNAME}" \
                            --annotation "manifest:org.opencontainers.image.description=Containerized version of ${PROJECTNAME}" \
                            --annotation "manifest:org.opencontainers.image.licenses=MIT" \
                            --annotation "manifest:org.opencontainers.image.version=${VERSION}" \
                            --annotation "manifest:org.opencontainers.image.created=${BUILD_DATE}" \
                            --annotation "manifest:org.opencontainers.image.revision=${COMMIT_ID}" \
                            --annotation "manifest:org.opencontainers.image.url=https://${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}" \
                            --annotation "manifest:org.opencontainers.image.source=https://${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}" \
                            --annotation "manifest:org.opencontainers.image.documentation=https://${GIT_FQDN}/${PROJECTORG}/${PROJECTNAME}" \
                            ${tags} \
                            --push \
                            .
                    """
                }
            }
        }
    }

    post {
        always {
            cleanWs()
        }
    }
}
