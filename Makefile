# --- Override repo-operator defaults ---
override OPERATOR_PATH     := build/repo-operator
override DEFAULT_BRANCH    := main
override OPENAPI_FILE      := api/OpenAPI/openapi.yaml
override OPENAPI_DOCS_PATH := docs/api

# --- Include the modules you need ---
include ${OPERATOR_PATH}/makefiles/base.mk
#include ${OPERATOR_PATH}/makefiles/golang.mk
#include ${OPERATOR_PATH}/makefiles/openapi.mk
#include ${OPERATOR_PATH}/makefiles/package.mk
#include ${OPERATOR_PATH}/makefiles/security.mk
include ${OPERATOR_PATH}/makefiles/changelog.mk
