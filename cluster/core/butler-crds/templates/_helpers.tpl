{{- define "butler-crds.statusSchema" -}}
type: object
properties:
  observedGeneration:
    type: integer
    format: int64
  providerID:
    type: string
  conditions:
    type: array
    x-kubernetes-list-type: map
    x-kubernetes-list-map-keys: [type]
    items:
      type: object
      required: [type, status, reason, message, lastTransitionTime]
      properties:
        type:
          type: string
        status:
          type: string
          enum: ["True", "False", "Unknown"]
        observedGeneration:
          type: integer
          format: int64
        reason:
          type: string
        message:
          type: string
        lastTransitionTime:
          type: string
          format: date-time
{{- end }}
