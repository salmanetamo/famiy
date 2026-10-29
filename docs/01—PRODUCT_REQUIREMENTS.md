# Product Requirements — Family Graph

## Vision
An interactive web application for exploring, visualizing, and managing family trees.

## Core features
- Person profiles with names, dates, photographs, biographies, and residence information.
- Family relationships with types and descriptions.
- Interactive graph with panning, zooming, node selection, and centering.
- Generational layout and age-based ordering where feasible.
- Search and navigation.
- Person and relationship management.
- Authentication, authorization, privacy, and data integrity.

## Key constraints
- Family structures may contain multiple partnerships, blended families, and incomplete historical information.
- Distinguish relationship type, genealogical distance, and emotional closeness.
- Keep domain data separate from visual layout state.
- Support an incremental MVP and evaluate performance at increasing graph sizes.

## Open questions
- Single-user or collaborative family trees?
- Initial target size of a family network?
- Required import/export formats?
- Preferred hosting budget?
- Privacy and data retention requirements?

## Definition of done
A deployed, tested, maintainable application with correct relationship modeling, an intuitive graph interface, and documented technical decisions.