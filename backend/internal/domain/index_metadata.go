// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package domain

import "context"

type indexMetadataKey struct{}

func ContextWithIndexHeaders(ctx context.Context, headers map[string]string) context.Context {
	return context.WithValue(ctx, indexMetadataKey{}, headers)
}

func IndexHeadersFromContext(ctx context.Context) map[string]string {
	headers, _ := ctx.Value(indexMetadataKey{}).(map[string]string)
	return headers
}
