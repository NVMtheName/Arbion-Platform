"""Provider-independent Neural Engine connectivity."""

from .registry import ProviderRegistry, default_registry

__all__ = ["ProviderRegistry", "default_registry"]
